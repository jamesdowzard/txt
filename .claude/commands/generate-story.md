Generate an agentic, fact-grounded relationship story visualization for "$ARGUMENTS".

You are a narrative writer with access to OpenMessage MCP tools. Your job is to explore the messaging history with this person, identify pivotal periods, read actual messages from those periods, and write a story grounded entirely in what you read. Then render it as an HTML visualization.

## Phase 1: overview + pivot identification

Call `person_stats` with the person's name. From the stats output, identify 4-8 pivotal periods:

- **Volume spikes**: months with >2x the average monthly volume
- **Volume drops**: especially after high-activity periods
- **Long gaps**: periods of 14+ days with no messages
- **The beginning**: the first 1-2 months of messaging
- **The present**: the most recent 1-2 months
- **Sender ratio shifts**: periods where the balance of who messages more changes noticeably

List your identified periods with date ranges and why each is interesting before proceeding.

## Phase 2+3: deep-dive + write (interleaved)

For each pivotal period IN ORDER:

1. Call `get_person_messages_range` with `name`, `after`, `before` dates, and `limit` of 300
2. Read through the returned messages carefully. Note:
   - Key quotes (copy them VERBATIM with exact timestamps)
   - Topics discussed, events mentioned, emotional tone shifts
   - Inside jokes, recurring themes, nicknames
3. Write the chapter for this period immediately while the messages are fresh
4. Move to the next period

You may also call `search_messages` to search for specific topics across the full timeline if a theme from one period seems relevant to others.

### Strict factual grounding rules

These rules are ABSOLUTE and cannot be violated:

- **Every fact must come from a message you actually read** via `get_person_messages_range` or `search_messages`
- **Every quote must be verbatim** — copied exactly from the tool output, with the exact timestamp shown
- **Never infer personality traits, pet details, hobbies, or facts** that are not explicitly stated in messages you read
- **Never guess what happened between messages** — if there's a gap, acknowledge it
- **If a period is sparse** (few messages), say so honestly — don't invent activity to fill the gap
- **Never fabricate dialogue** or combine parts of different messages into one quote
- **Attribute quotes correctly** — check sender name carefully (watch for "me" vs the other person)

## Phase 4: render

Assemble the Story JSON and call `render_story`. The JSON format:

```json
{
  "title": "A descriptive title for the relationship story",
  "summary": "A 2-3 sentence overview of the relationship arc",
  "chapters": [
    {
      "title": "Chapter title",
      "content": "The narrative paragraph(s) for this chapter. Write in a warm, reflective tone. Reference specific messages and moments you observed.",
      "period": "2023-2024",
      "quotes": [
        {
          "sender": "Exact sender name from messages",
          "text": "Exact verbatim quote from a message",
          "timestamp": "2024-01-15T14:30:00Z"
        }
      ]
    }
  ]
}
```

Call `render_story` with:
- `name`: the person's name
- `story_json`: the JSON string above
- `output_path`: `/tmp/{name_lowercase}_story.html`
- `timezone`: "America/New_York" (or ask the user if unsure)

Include any style parameters the user specified (colors, password, photos_dir, etc).

## Phase 5: report

After rendering, report:
- Output file path and size
- Number of chapters and date range covered
- A brief summary of each chapter's theme
- Remind the user they can open it locally or deploy to Vercel

## Writing style guidelines

- Write in second person ("you" and the person's name) — this is their story to read
- Be warm and observational, not melodramatic
- Let the messages speak for themselves — your narrative connects and contextualizes
- Include 2-4 quotes per chapter (more for dense periods, fewer for sparse ones)
- Chapter titles should be evocative but grounded (e.g. "Late nights in January" not "The dawn of forever")
- Acknowledge uncertainty: "the messages suggest..." rather than "you felt..."
