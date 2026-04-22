# Project: TGFileBot

High-performance Telegram Bot & UserBot integration for media streaming, link extraction, and group management.

## Vision
An open-source, high-performance tool that turns Telegram into a personal media server with direct link access and robust management features.

## Core Problem
- Difficulty in direct streaming/downloading Telegram media without the official client.
- Lack of centralized API for searching and extracting links from multiple channels.
- Need for simple, automated group moderation (keyword/regex filtering).

## Target Audience
- Personal use (Private cloud enthusiasts).

## Tech Stack
- **Language**: Go
- **Library**: `github.com/amarnathcjd/gogram/telegram` (UserBot + Bot API)
- **Deployment**: Docker / Docker Compose
- **Features**: Stream (分片流), Search (API), Remote Management, Group Moderation (Planned).

## Key Pain Points
1. **Initial Connection Timeout**: First play after inactivity often timeouts. Subsequent plays are fast. Potential logic, DNS, or cold-start issue.
2. **Missing Group Management**: Need keyword/regex-based message deletion for group moderation.

## Success Metrics
- 100% reliability in initial video playback (no timeout).
- Fully functional keyword/regex moderation commands (`/add`, `/del`, `/list`).
