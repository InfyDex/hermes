# Setting Up Discord Notifications

Hermes can send real-time alerts to your Discord server whenever a job fails, hangs, or when the system restarts. This gives you instant visibility into your server environments directly from your phone or desktop.

## Steps to Obtain a Webhook URL

1. **Open Discord** and navigate to your desired server where you have administrative permissions.
2. Click the Server Name at the top left and select **Server Settings**.
3. Go to **Integrations** > **Webhooks**.
4. Click **New Webhook**. 
5. Customize the Name (e.g., "Hermes Alerts", "Homelab Scheduler") and select the Channel where you want the notifications to appear.
6. Click **Copy Webhook URL** and then **Save Changes**.

## Configuring Hermes

Simply pass the generated webhook URL into Hermes via environment variables in your `docker-compose.yml` or `.env` system:

```yaml
environment:
  - HERMES_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/xxxx/xxxx
```

Once you restart the Hermes container, it will automatically broadcast a "System Ready" message to the channel confirming that the connection is active!