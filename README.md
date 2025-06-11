# Debt Tracking Telegram Bot

A Telegram bot that helps track debts between users in a group chat.

## Setup

1. Create a new bot using [@BotFather](https://t.me/botfather) on Telegram
2. Get your bot token
3. Install Go dependencies:
   ```bash
   go mod tidy
   ```
4. Run the bot:
   ```bash
   TELEGRAM_BOT_TOKEN=your_token go run main.go
   ```

## Usage

1. Add the bot to your group chat
2. Use the following format to record a debt:
   ```
   @username amount [reason]
   ```
   For example:
   ```
   @john 50 lunch
   ```
