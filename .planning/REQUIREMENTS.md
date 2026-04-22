# Requirements: TGFileBot

## V1: Current Milestone (Fix & Moderation)

### Bug Fixes
- **[BUG-001]**: Resolve initial connection timeout in `/stream`.
  - First play after long inactivity must start within 5s.
  - Investigate DNS, TCP keep-alive, and Telegram `file_reference` refresh logic on cold start.

### Group Moderation
- **[FEAT-001]**: Message Monitoring.
  - Bot must monitor all messages in groups where it is an admin.
- **[FEAT-002]**: Keyword/Regex Deletion.
  - Delete messages matching configured keywords or regular expressions.
  - Requires `delete_messages` permission.
- **[FEAT-003]**: Moderation Commands.
  - `/addRule <regex>`: Add a deletion rule.
  - `/delRule <index/regex>`: Remove a deletion rule.
  - `/list`: Integrated view of channels, IDs, and rules.

## V2: Future
- Web Dashboard for management.
- Multi-user authentication for API.
- Support for more media types.

## Out of Scope
- Full Telegram client features (chatting, etc).
- Public deployment hosting.
