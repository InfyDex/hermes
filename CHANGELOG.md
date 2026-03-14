# Changelog - v0.0.3

## ✨ New Features
* **Multi-Channel Notification Engine:** Comprehensive rewrite introducing alerts for Discord Webhooks, SMTP Email integrations, and In-App Web UI drop-down alerts.
* **Job Enable/Disable Toggles:** Added quick-action UI buttons on the Dashboard and Job Detail pages to easily Disable running schedule formulas without resorting to deleting configurations entirely.
* **Server Branding (`HERMES_SERVER_NAME`):** New environment variable to categorize output logs for users running multiple distinct Hermes containers. Appends tags gracefully onto SMTP Email Subject headers and Discord bot usernames natively (e.g. `[Hermes - Edith]`).
* **Environment-Native Architectures:** Deprecated file-dependent `config.yaml` structs entirely. Moving forward, the standard image operates strictly via runtime `HERMES_*` Environment Variables securely.
* **System Event Notifications:** Emits standard infrastructure broadcast over Discord/Emails specifically notifying administrators seamlessly during background application reboot cycles indicating total jobs initialized.

## 🐛 Bug Fixes
* **Container Timezone Resolution:** Fixed frontend dashboard HTML executing under unformatted UTC structures. The dashboard logs now securely obey your respective local `TZ=` container mappings.  
* **Static Assets 404 Routing:** Updated Golang's native `//go:embed` handler routing arrays to securely load standard brand favicon metadata headers into memory.
* **Docker Port Standard:** Rewired the default base Web UI port to target `4376` eliminating the common 8080 collision matrix.

## 📖 Documentation
* Generated local testing container workflow documentation in standard `.gitignore` formatting structures.
* Implemented new specialized `docs/discord-setup.md` instructions.
* Implemented new specialized `docs/email-setup.md` credential management instructions.