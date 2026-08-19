# Private File Share

Send files to a nearby phone, across this Wi-Fi, or anywhere on the
internet — no account, no upload to a server this app runs itself.
Opens as its own window.

## Modes

**Share nearby** — starts a small HTTP server on your LAN and shows a QR
code. Anything on the same Wi-Fi (a phone's browser, another computer)
can scan it or open the link directly — no app to install on their end.
Choose individual files or a whole folder (browsable, subfolders and
all). Nothing leaves your network.

**Send anywhere** — works across the internet, not just this Wi-Fi.
Relayed through [croc](https://github.com/schollz/croc)'s public relay,
which only ever sees end-to-end-encrypted ciphertext — never your file
contents. Gives you a short code phrase to share with the recipient.

**Receive a file** — enter a code someone gave you. Compatible with the
real `croc` CLI too: a recipient without this app can receive with
`croc <code>` directly, since it's the same protocol.

## Requirements

**A Chromium-based browser already installed**: Google Chrome, Chromium,
Brave, Microsoft Edge, or Arc — renders the app's own UI window.

## Notes

- "Send anywhere" resolves its relay address once, at startup — if you
  open this app with no internet connection and connect later, restart
  it before using that mode.
- Received files always save to your Downloads folder.

## License

MIT — see [LICENSE](LICENSE).
