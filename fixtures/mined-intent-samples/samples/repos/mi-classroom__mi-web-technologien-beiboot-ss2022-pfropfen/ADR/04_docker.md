# Docker

## Status

Accepted

## Context

Die Website soll in einem Docker-Container laufen. Dazu muss ein entsprechendes Docker Image im Docker Hub öffentlich bereitgestellt werden.
Es besteht die Möglichkeit den Container 'standalone' per run-Befehl zu starten oder Docker-Compose mit einer vorkonfigurierten yml-Datei zu verwenden.
Das benötigte Image wird bei Bedarf aus dem Docker Hub geladen.

## Decision

Der Docker Service wird nicht per Docker-Compose ausgerollt, sondern per run-Befehl gestartet.
Solange es sich nur um 1 Container handelt, scheint Compose nicht notwendig zu sein. 
## Consequences

Der run-Befehl von Docker benötigt zusätzliche flags für den Namen sowie die Ports bzw die Portweiterleitung.
