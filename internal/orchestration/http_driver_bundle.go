package orchestration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const httpDriverBundleLimit int64 = 32 << 20
const httpDriverTransferTimeout = 2 * time.Minute
const httpDriverGitTimeout = 2 * time.Minute
const httpDriverPackObjectLimit = 10000

var errHTTPDriverBundle = errors.New("HTTP dispatch result bundle is invalid or could not be independently verified")

type httpDriverBundleFact struct {
	Artifact   ArtifactRef `json:"artifact"`
	Size       int64       `json:"size_bytes"`
	ResultTree string      `json:"result_tree"`
}

func (t *httpDispatchTransport) downloadBundle(ctx context.Context, p httpDriverPending, artifact *httpDriverBundleArtifact, destination *os.File) error {
	if artifact == nil || artifact.URL != t.base+"/dispatches/"+p.ID+"/artifacts/result-bundle" || artifact.Size < 1 || artifact.Size > httpDriverBundleLimit {
		return errHTTPDriverBundle
	}
	deadline := time.Now().Add(httpDriverTransferTimeout)
	if p.ExpiresAt.Before(deadline) {
		deadline = p.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if t.authorize != nil {
		if err := t.authorize(ctx); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return errHTTPDriverBundle
	}
	req.Header.Set("Authorization", t.bearer)
	req.Header.Set("Accept", "application/x-git-bundle")
	req.Header.Set("Accept-Encoding", "identity")
	res, err := t.client.Do(req)
	if err != nil {
		return httpDispatchExchangeError(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &httpDispatchError{Status: res.StatusCode}
	}
	media, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || media != "application/x-git-bundle" || len(res.Header.Values("Content-Type")) != 1 ||
		len(res.Header.Values("Content-Encoding")) > 1 || (res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity") ||
		(res.ContentLength >= 0 && res.ContentLength != artifact.Size) {
		return errHTTPDriverBundle
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(res.Body, artifact.Size+1))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return httpDispatchExchangeError(err)
	}
	if n != artifact.Size || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return errHTTPDriverBundle
	}
	if err := destination.Sync(); err != nil {
		return errHTTPDriverState
	}
	return nil
}

// Only the strict v2 one-prerequisite/one-head form is supported. Read no more
// than 8 KiB of header; the pack itself is validated by Git in quarantine.
func httpDriverBundleHeader(file *os.File, source, result, id string) (int64, error) {
	if !httpDriverSHA1.MatchString(source) || !httpDriverSHA1.MatchString(result) || !httpDispatchID.MatchString(id) {
		return 0, errHTTPDriverBundle
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, errHTTPDriverBundle
	}
	reader := bufio.NewReader(io.LimitReader(file, httpDispatchSmallBody))
	line, err := reader.ReadString('\n')
	if err != nil || line != "# v2 git bundle\n" {
		return 0, errHTTPDriverBundle
	}
	offset := int64(len(line))
	line, err = reader.ReadString('\n')
	if err != nil || len(line) > 4096 || !strings.HasPrefix(line, "-"+source+" ") || !utf8.ValidString(line) ||
		strings.ContainsFunc(strings.TrimSuffix(line, "\n"), func(r rune) bool { return unicode.IsControl(r) }) {
		return 0, errHTTPDriverBundle
	}
	offset += int64(len(line))
	line, err = reader.ReadString('\n')
	if err != nil || line != result+" refs/dispatch/"+id+"\n" {
		return 0, errHTTPDriverBundle
	}
	offset += int64(len(line))
	line, err = reader.ReadString('\n')
	if err != nil || line != "\n" {
		return 0, errHTTPDriverBundle
	}
	offset++
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, errHTTPDriverBundle
	}
	var signature [12]byte
	if _, err := io.ReadFull(file, signature[:]); err != nil || string(signature[:4]) != "PACK" {
		return 0, errHTTPDriverBundle
	}
	version, count := binary.BigEndian.Uint32(signature[4:8]), binary.BigEndian.Uint32(signature[8:12])
	if (version != 2 && version != 3) || count < 1 || count > httpDriverPackObjectLimit {
		return 0, errHTTPDriverBundle
	}
	return offset, nil
}

func (d *httpDriver) verifyBundle(ctx context.Context, p httpDriverPending, transport *httpDispatchTransport, receipt httpDriverReceipt, accepted *httpDispatchHandle, status *httpDispatchStatus) (*httpDispatchAdmittedReceipt, *httpDriverBundleFact, error) {
	if !p.Trust.AllowEdits || p.Trust.ReadOnly || receipt.Bundle == nil || receipt.Wire.Repository.ResultCommit == nil || receipt.Wire.Execution.State != "succeeded" {
		return nil, nil, errHTTPDriverEdited
	}
	dir, err := d.stateDir(true)
	if err != nil {
		return nil, nil, err
	}
	defer dir.Close()
	stage := ".quarantine-" + rand.Text()
	if err := dir.Mkdir(stage, 0o700); err != nil {
		return nil, nil, errHTTPDriverState
	}
	defer dir.RemoveAll(stage)
	root, err := dir.OpenRoot(stage)
	if err != nil {
		return nil, nil, errHTTPDriverState
	}
	defer root.Close()
	file, err := root.OpenFile("result.bundle", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil, errHTTPDriverState
	}
	defer file.Close()
	if err := transport.downloadBundle(ctx, p, receipt.Bundle, file); err != nil {
		return nil, nil, err
	}
	result := *receipt.Wire.Repository.ResultCommit
	offset, err := httpDriverBundleHeader(file, p.Trust.SourceCommit, result, p.ID)
	if err != nil {
		return nil, nil, err
	}
	stagePath, err := httpDriverResolvedPath(filepath.Join(d.store.Root, httpDriverStateDir, stage))
	if err != nil {
		return nil, nil, errHTTPDriverState
	}
	ctx, cancel := context.WithTimeout(ctx, httpDriverGitTimeout)
	defer cancel()
	runner, quarantine, err := newHTTPDriverQuarantine(ctx, stagePath, p.Checkout, p.Trust.SourceCommit)
	if err != nil {
		return nil, nil, err
	}
	bundlePath := filepath.Join(stagePath, "result.bundle")
	if _, err := runner.checked(ctx, quarantine, "bundle", "verify", bundlePath); err != nil {
		return nil, nil, err
	}
	heads, err := runner.checked(ctx, quarantine, "bundle", "list-heads", bundlePath)
	if err != nil || string(heads) != result+" refs/dispatch/"+p.ID+"\n" {
		return nil, nil, errHTTPDriverBundle
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, nil, errHTTPDriverBundle
	}
	if err := runner.stream(ctx, quarantine, file, io.Discard, "index-pack", "--stdin", "--fix-thin", "--strict"); err != nil {
		return nil, nil, err
	}
	if err := verifyHTTPDriverCommit(ctx, runner, quarantine, p.Trust.SourceCommit, result, receipt.Tree); err != nil {
		return nil, nil, err
	}
	if _, err := runner.checked(ctx, quarantine, "fsck", "--full", "--strict", "--no-reflogs", "--no-dangling", p.Trust.SourceCommit, result); err != nil {
		return nil, nil, err
	}
	ref := "refs/dispatch/" + p.ID
	if _, err := runner.checked(ctx, quarantine, "-c", "protocol.file.allow=always", "fetch", "--no-write-fetch-head", "--no-tags", "--no-recurse-submodules", "--no-auto-maintenance", bundlePath, ref+":"+ref); err != nil {
		return nil, nil, err
	}
	if err := verifyHTTPDriverExactPaths(ctx, runner, quarantine, p.Trust.SourceCommit, result, receipt.Wire); err != nil {
		return nil, nil, err
	}
	admitted, err := admitHTTPDispatchReceipt(ctx, runner, quarantine, p.Frozen, accepted, status, receipt.Base, nil)
	if err != nil {
		return nil, nil, err
	}
	name := p.ID + ".result.bundle"
	if err := dir.Link(filepath.Join(stage, "result.bundle"), name); err != nil {
		if !errors.Is(err, os.ErrExist) || !httpDriverSameBundle(dir, name, receipt.Bundle) {
			return nil, nil, errHTTPDriverState
		}
	}
	fact := &httpDriverBundleFact{Artifact: ArtifactRef{Kind: "path", Value: filepath.Join(d.store.Root, httpDriverStateDir, name), SHA256: receipt.Bundle.SHA256}, Size: receipt.Bundle.Size, ResultTree: receipt.Tree}
	return admitted, fact, nil
}

func httpDriverSameBundle(dir *os.Root, name string, artifact *httpDriverBundleArtifact) bool {
	info, err := dir.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != artifact.Size {
		return false
	}
	file, err := dir.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, artifact.Size+1))
	return err == nil && n == artifact.Size && "sha256:"+hex.EncodeToString(hash.Sum(nil)) == artifact.SHA256
}

func verifyHTTPDriverCommit(ctx context.Context, runner *httpDriverGit, quarantine, source, result, tree string) error {
	raw, err := runner.checked(ctx, quarantine, "cat-file", "commit", result)
	if err != nil {
		return err
	}
	header, _, ok := bytes.Cut(raw, []byte("\n\n"))
	if !ok {
		return errHTTPDriverBundle
	}
	parents, trees := 0, 0
	for _, line := range strings.Split(string(header), "\n") {
		if strings.HasPrefix(line, "parent ") {
			parents++
			if line != "parent "+source {
				return errHTTPDriverBundle
			}
		}
		if strings.HasPrefix(line, "tree ") {
			trees++
			if line != "tree "+tree {
				return errHTTPDriverBundle
			}
		}
	}
	if parents != 1 || trees != 1 {
		return errHTTPDriverBundle
	}
	return nil
}

func verifyHTTPDriverExactPaths(ctx context.Context, runner *httpDriverGit, quarantine, source, result string, wire httpDispatchReceiptAdmissionWire) error {
	if !wire.Evidence.Complete {
		return errHTTPDispatchPaths
	}
	claims := make(map[string]bool)
	for _, name := range wire.Evidence.Edited {
		if claims[name] {
			return errHTTPDispatchPaths
		}
		claims[name] = true
	}
	diff, err := runner.checked(ctx, quarantine, "diff-tree", "-r", "--no-commit-id", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-relative", "--ignore-submodules=none", "--name-only", "-z", source, result, "--")
	if err != nil {
		return err
	}
	for _, name := range bytes.Split(diff, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if !claims[string(name)] {
			return errHTTPDispatchPaths
		}
		delete(claims, string(name))
	}
	if len(claims) != 0 {
		return errHTTPDispatchPaths
	}
	return nil
}
