# arXiv weekly

Recommends the five AI papers of the week worth reading, by Telegram.

It works in two steps. `./run.sh prepare` downloads the papers of the last closed arXiv week in cs.AI, cs.LG and cs.CL, asks a language model to shortlist them by title, then to pick five from the abstracts of the shortlist, and stores the message. `./run.sh send` sends the stored message. What counts as relevant is described in `intereses.md`: edit it to get your own picks.

The model is reached through an external `llm` command, so this repo has no model code or API keys. Any command works if it follows this interface:

- Arguments: `--sistema @FILE` (system prompt in a file), `--esquema JSON` (JSON Schema the answer must follow), `--nivel mejor`, `--esfuerzo bajo|medio|alto`, `--sin-apuro` (it may take hours, for example through a batch API), `--max-tokens N` and `--timeout SECONDS`.
- Input: the user message on stdin.
- Output: only the JSON answer on stdout, valid against the schema; a non-zero exit code on failure.

`LLM_BIN` in `.env` says where the command is; without it, `llm` is looked up in the PATH.

## Setup

1. Copy `.env.example` to `.env` and fill in the values. `.env` is gitignored. Make sure an `llm` command is available (see above).
2. Send `/start` to the bot from the chat that will receive the messages.
3. Run `./run.sh prepare` (from cron, Sundays at 0:23, 4:23 and 7:23). It prints the message and stores it in `pending-message.html`. Once this week's message is stored, or while another run holds `prepare.lock`, it does nothing, so the later runs only matter when arXiv failed the earlier ones. A failure is reported to the same Telegram chat; `--no-alert` silences it for the runs that a later one will retry.
4. Run `./run.sh send` (from cron, Sundays at 9).

## License

MIT. See `LICENSE`.
