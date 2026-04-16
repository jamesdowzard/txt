// Textbridge web entry point.
//
// Phase 0: lift-and-shift — the legacy monolith's CSS + JS are loaded here.
// Subsequent phases (see docs/plans/2026-04-17-textbridge-roadmap.md Task 0.4+)
// peel chunks of legacy.js into proper Preact components.
import 'uno.css';
import './styles/tokens.css';
import './styles/legacy.css';
import './legacy.js';
