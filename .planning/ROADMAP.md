# Roadmap: TGFileBot

## Milestone 1: Stability & Moderation

| Phase | Goal | Requirements | Success Criteria |
|-------|------|--------------|------------------|
| 1 | Debug Playback | [BUG-001] | First play starts <5s after 1h idle. |
| 2 | Core Moderation | [FEAT-001], [FEAT-002] | Messages with "spam" auto-deleted. |
| 3 | Moderation UI | [FEAT-003] | Rules manageable via bot commands. |

---

### Phase 1: Debug Playback
Goal: Eliminate the "First Play Timeout" issue.
Requirements: [BUG-001]
Success criteria:
1. Identify root cause (DNS vs Logic vs Network).
2. Implement fix (Pre-warm connection or optimized retry).
3. Verified by idle test (>1h).

### Phase 2: Core Moderation
Goal: Enable bot to auto-delete unwanted content.
Requirements: [FEAT-001], [FEAT-002]
Success criteria:
1. Message handler captures group text.
2. Regex engine matches "test_spam".
3. Message successfully deleted by Bot.

### Phase 3: Moderation UI
Goal: Add commands for user to manage rules.
Requirements: [FEAT-003]
Success criteria:
1. `/addRule` persists data.
2. `/list` shows rules alongside channels/IDs.
3. `/delRule` removes specific entry.
