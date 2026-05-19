# Product Requirements Document (PRD)

**Product:** MALD (Markdown / AI / Local / Daemon)
**Vision:** The ultimate local-first knowledge base that blends VS Code's power with Obsidian's linking and local AI intelligence.

## 1. Core Features

### Knowledge Management (The "Obsidian" part)
- **Local Markdown:** All data stored as plain `.md` files on disk. Zero lock-in.
- **Bi-directional Linking:** `[[WikiLinks]]` support.
- **Backlinks:** Panel showing what links to the current note.
- **Graph View:** Interactive visualization of note connections.
- **Tags:** `#tag` support with indexing.

### Editor Experience (The "VS Code" part)
- **Syntax Highlighting:** Full support for Markdown, Code blocks (Rust, Python, JS, etc.).
- **Tabs:** Multiple documents open at once.
- **Split Views:** Edit + Preview side-by-side.
- **Command Palette:** `Ctrl+Shift+P` access to all actions.
- **Terminal:** Integrated terminal for system commands.

### Local AI (The "Brain")
- **Ollama Integration:** Connects to local LLMs (Llama 3, Mistral, etc.).
- **RAG (Retrieval Augmented Generation):** Chat with your notes.
  - Indexing: HNSW vector store for semantic search.
  - Context: Chat knows your vault content.
- **Inline Assist:** "Fix grammar", "Summarize", "Generate code" directly in editor.

### Task Management
- **Task Parsing:** `- [ ]` items parsed from all files.
- **Kanban Board:** Visual management of tasks by status (Todo/Doing/Done).

### Architecture / Sync
- **Daemon Mode:** Background service for indexing/watching file changes.
- **Sync:** Git-based or proprietary sync logic (implied by `sync.rs`).

## 2. Technical Constraints
- **Performance:** GUI must run at 60fps.
- **Startup:** < 500ms.
- **Memory:** Efficient resource usage (Rust).
- **Offline:** 100% functional without internet.
