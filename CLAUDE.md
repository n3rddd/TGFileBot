# TGFileBot Guide

Project focusing on high-performance Telegram streaming and management.

## GSD Workflow
- **Commands**:
  - `/gsd-discuss-phase <N>`: Start contextual discussion.
  - `/gsd-plan-phase <N>`: Create execution plan.
  - `/gsd-execute-phase <N>`: Run the plan.
  - `/gsd-progress`: Check current state.

## Tech Stack
- Go (gogram library)
- Docker
- Producer-Consumer model for streaming.

## Project Structure
- `.planning/`: Project memory and roadmap.
- `main.go`: Entry point.
- `stream.go`: Streaming logic (Producer-Consumer).
- `command.go`: Bot commands.
- `http.go`: API handlers.

## Next Steps
1. Run `/gsd-discuss-phase 1` to debug the playback timeout.
2. Implement Group Management features.
