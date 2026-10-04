# arXiv weekly

Recommends the five AI papers of the week worth reading, by Telegram.

It works in two steps. `./run.sh prepare` downloads the papers of the last closed arXiv week in cs.AI, cs.LG and cs.CL, asks Claude to shortlist them by title, then to pick five from the abstracts of the shortlist, and stores the message. `./run.sh send` sends the stored message. Claude answers through the Batches API, at half the price, so `prepare` can take from minutes to hours. What counts as relevant is described in `intereses.md`.

## Setup

1. Copy `.env.example` to `.env` and fill in the values. `.env` is gitignored.
2. Send `/start` to the bot from the chat that will receive the messages.
3. Run `./run.sh prepare` (from cron, Sundays at 0). It prints the message and stores it in `pending-message.html`.
4. Run `./run.sh send` (from cron, Sundays at 9).
