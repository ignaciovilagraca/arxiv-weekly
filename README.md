# arXiv weekly

Recommends the five AI papers of the week worth reading, by Telegram.

Every Saturday it downloads the papers of the last closed arXiv week in cs.AI, cs.LG and cs.CL, asks Claude to shortlist them by title, then to pick five from the abstracts of the shortlist. What counts as relevant is described in `intereses.md`.

## Setup

1. Copy `.env.example` to `.env` and fill in the values. `.env` is gitignored.
2. Send `/start` to the bot from the chat that will receive the messages.
3. Run `./run.sh --dry-run` to print the recommendation without sending it.
4. Run `./run.sh` (from cron, Saturdays at 9).
