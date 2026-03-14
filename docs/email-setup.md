# Setting Up Email Notifications (SMTP)

Hermes can deliver failed job alerts directly to your inbox using any standard SMTP server, including personal Gmail or custom domains. When long-running scripts stall or processes break, you'll receive the direct log securely over an email.

## Generating an App Password (For Gmail)

If you are using a standard Gmail account or Google Workspace, you must generate an "App Password" to allow automated emailing rather than exposing your real account password.

1. Go to your **Google Account** settings: [myaccount.google.com](https://myaccount.google.com/).
2. Navigate to the **Security** tab.
3. Under "How you sign in to Google", ensure **2-Step Verification** is turned on.
4. Click on **2-Step Verification** and scroll down to **App Passwords**.
5. Create a new App Password named "Hermes".
6. Specify the generated 16-character password in your configuration (spaces are not required).

## Configuring Hermes

Add your standard SMTP provider credentials to your Hermes `docker-compose.yml`:

```yaml
environment:
  - HERMES_SMTP_HOST=smtp.gmail.com
  - HERMES_SMTP_PORT=587
  - HERMES_SMTP_USER=your-email@gmail.com
  - HERMES_SMTP_PASS=your-16-character-app-password
  - HERMES_SMTP_FROM=your-email@gmail.com
```

**Common Ports Defaults:**
- **TLS/STARTTLS:** `587` (Most common for modern providers like Gmail/Outlook)
- **SSL:** `465`
- **Unencrypted:** `25` (Not recommended)

Once you restart your Hermes container, it will automatically shoot an initialization confirmation email verifying that the SMTP connection works!