# Debt Tracking Telegram Bot

A Telegram bot that helps track debts between users in a group chat.

## Features

- Works only in group chats
- Record debts using the format `@username amount [reason]`
- View all debts in the current chat
- View net balances between users (debts cancel each other out)
- Simple command interface
- In-memory debt tracking

## Setup

1. Create a new bot using [@BotFather](https://t.me/botfather) on Telegram
2. Get your bot token
3. Replace `YOUR_BOT_TOKEN` in `main.go` with your actual bot token
4. Install Go dependencies:
   ```bash
   go mod tidy
   ```
5. Run the bot:
   ```bash
   go run main.go
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

## Commands

- `/start` - Get started with the bot
- `/help` - Show help message
- `/debts` - Show all current debts in the chat
- `/balance` - Show net balances between users (debts cancel each other out)

## Note

This is a simple implementation that stores debts in memory. In a production environment, you would want to:
- Use a database to persist the debts
- Add commands to view and settle debts
- Add user authentication
- Add more error handling
- Add commands to settle debts
- Add commands to view debts for specific users 