package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
)

// Debt represents a debt between two users
type Debt struct {
	From    string
	To      string
	Amount  float64
	Reason  string
	ChatID  int64
	Time    time.Time
}

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./debts.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create debts table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS debts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_user TEXT NOT NULL,
			to_user TEXT NOT NULL,
			amount REAL NOT NULL,
			reason TEXT,
			chat_id INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			operation_type TEXT DEFAULT 'debt'
		)
	`)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// Get bot token from environment variable
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	// Initialize database
	initDB()
	defer db.Close()

	// Create bot instance
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Check if the message is from a group chat
		if update.Message.Chat.Type == "private" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Этот бот работает только в групповых чатах. Пожалуйста, добавьте меня в групповой чат!")
			bot.Send(msg)
			continue
		}

		// Handle commands
		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			
			switch update.Message.Command() {
			case "help":
				msg.Text = `Как пользоваться ботом:

1. Запись долга:
   • @username сумма [причина] - записать долг для одного человека
   • @user1 @user2 сумма [причина] - разделить сумму между несколькими людьми
   • @all сумма [причина] - разделить сумму между всеми участниками чата

2. Команды:
   • /balance - показать все долги в чате
   • /balance me - показать ваши личные долги
   • /history [дней] - показать историю операций (по умолчанию за 1 день)
   • /help - показать это сообщение

Примеры:
• @ivan 50 обед
• @ivan @maria 100 ужин
• @all 150 вечеринка
• /history 30 - показать историю за 30 дней`
			case "balance":
				// Calculate and show net balances
				chatDebts := getChatDebts(update.Message.Chat.ID)
				if len(chatDebts) == 0 {
					msg.Text = "В этом чате пока нет записанных долгов."
				} else {
					// Create a map to store net balances between users
					balances := make(map[string]map[string]float64)
					
					// Calculate all debts
					for _, debt := range chatDebts {
						// Initialize maps if they don't exist
						if _, exists := balances[debt.From]; !exists {
							balances[debt.From] = make(map[string]float64)
						}
						if _, exists := balances[debt.To]; !exists {
							balances[debt.To] = make(map[string]float64)
						}
						
						// Add the debt (To owes From)
						balances[debt.To][debt.From] += debt.Amount
						balances[debt.From][debt.To] -= debt.Amount
					}
					
					// Build the response
					var response strings.Builder
					
					// Check if this is a personal balance request
					args := update.Message.CommandArguments()
					isPersonal := args == "me"
					
					if isPersonal {
						response.WriteString("Ваши долги:\n\n")
						author := update.Message.From.UserName
						
						// Track which pairs we've already processed
						processed := make(map[string]bool)
						hasDebts := false
						
						// Show non-zero balances involving the author
						for user1, debts := range balances {
							for user2, amount := range debts {
								// Skip if we've already processed this pair or if it's the same user
								pairKey := fmt.Sprintf("%s-%s", user1, user2)
								reversePairKey := fmt.Sprintf("%s-%s", user2, user1)
								if processed[pairKey] || processed[reversePairKey] || user1 == user2 {
									continue
								}
								
								// Only show balances involving the author
								if (user1 == author || user2 == author) && amount != 0 {
									hasDebts = true
									if amount > 0 {
										response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", user1, user2, amount))
									} else {
										response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", user2, user1, -amount))
									}
								}
								
								processed[pairKey] = true
								processed[reversePairKey] = true
							}
						}
						
						if !hasDebts {
							response.WriteString("У вас нет непогашенных долгов.")
						}
					} else {
						response.WriteString("Долги в этом чате:\n\n")
						
						// Track which pairs we've already processed
						processed := make(map[string]bool)
						
						// Show non-zero balances
						for user1, debts := range balances {
							for user2, amount := range debts {
								// Skip if we've already processed this pair or if it's the same user
								pairKey := fmt.Sprintf("%s-%s", user1, user2)
								reversePairKey := fmt.Sprintf("%s-%s", user2, user1)
								if processed[pairKey] || processed[reversePairKey] || user1 == user2 {
									continue
								}
								
								// Only show non-zero balances
								if amount != 0 {
									if amount > 0 {
										response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", user1, user2, amount))
									} else {
										response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", user2, user1, -amount))
									}
								}
								
								processed[pairKey] = true
								processed[reversePairKey] = true
							}
						}
						
						if response.Len() == len("Долги в этом чате:\n\n") {
							response.WriteString("Нет непогашенных долгов.")
						}
					}
					
					msg.Text = response.String()
				}
			case "history":
				// Get number of days from command arguments
				args := update.Message.CommandArguments()
				days := 1 // Default to 1 day if no argument provided
				if args != "" {
					if d, err := strconv.Atoi(args); err == nil && d > 0 {
						days = d
					}
				}

				// Get history for the specified period
				history, err := getDebtHistory(update.Message.Chat.ID, days)
				if err != nil {
					msg.Text = "Ошибка при получении истории. Пожалуйста, попробуйте снова."
					bot.Send(msg)
					continue
				}

				if len(history) == 0 {
					msg.Text = fmt.Sprintf("Нет операций за последние %d дней.", days)
				} else {
					var response strings.Builder
					response.WriteString(fmt.Sprintf("История операций за последние %d дней:\n\n", days))
					
					for _, debt := range history {
						// Get operation type for this debt
						var operationType string
						err := db.QueryRow(`
							SELECT operation_type 
							FROM debts 
							WHERE chat_id = ? AND from_user = ? AND to_user = ? AND amount = ? AND created_at = ?
						`, debt.ChatID, debt.From, debt.To, debt.Amount, debt.Time.Format("2006-01-02 15:04:05")).Scan(&operationType)
						if err != nil {
							log.Printf("Error getting operation type: %v", err)
							operationType = "debt" // Default to debt if there's an error
						}

						response.WriteString(fmt.Sprintf("[%s] ", debt.Time.Format("02.01.2006 15:04")))
						
						if operationType == "return" {
							response.WriteString(fmt.Sprintf("%s вернул %s %.2f", debt.From, debt.To, debt.Amount))
						} else {
							response.WriteString(fmt.Sprintf("%s должен %s %.2f", debt.To, debt.From, debt.Amount))
						}
						
						if debt.Reason != "" {
							response.WriteString(fmt.Sprintf(" %s", debt.Reason))
						}
						response.WriteString("\n")
					}
					msg.Text = response.String()
				}
			default:
				msg.Text = "Неизвестная команда"
			}

			bot.Send(msg)
			continue
		}

		// Handle debt messages
		text := update.Message.Text
		
		// First, check if it's an @all command
		allRe := regexp.MustCompile(`@all\s+(\d+(?:\.\d+)?)(?:\s+(.+))?`)
		allMatches := allRe.FindStringSubmatch(text)
		
		if allMatches != nil {
			// Get all chat administrators
			admins, err := bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
				ChatConfig: tgbotapi.ChatConfig{
					ChatID: update.Message.Chat.ID,
				},
			})
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при получении списка участников. Пожалуйста, попробуйте снова.")
				bot.Send(msg)
				continue
			}

			// Count active members (excluding bots)
			activeMembers := 0
			for _, admin := range admins {
				if !admin.User.IsBot {
					activeMembers++
				}
			}

			if activeMembers <= 1 {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Недостаточно участников в чате.")
				bot.Send(msg)
				continue
			}

			amount, _ := strconv.ParseFloat(allMatches[1], 64)
			reason := ""
			if len(allMatches) > 2 {
				reason = allMatches[2]
			}

			// Split amount between all members
			splitAmount := amount / float64(activeMembers) // Subtract 1 to exclude the author
			from := update.Message.From.UserName

			var response strings.Builder
			response.WriteString(fmt.Sprintf("Разделено %.2f между %d участниками (по %.2f каждый):\n", amount, activeMembers, splitAmount))

			// Create debts for each member
			for _, admin := range admins {
				if !admin.User.IsBot && admin.User.UserName != from {
					// Determine operation type and handle returns
					netBalance, err := getNetBalance(update.Message.Chat.ID, from, admin.User.UserName)
					if err != nil {
						log.Printf("Error getting net balance: %v", err)
						continue
					}

					if netBalance < 0 {
						// This is a return operation
						returnAmount := -netBalance // Convert negative balance to positive amount
						if splitAmount <= returnAmount {
							// Simple return - amount is less than or equal to existing debt
							debt := Debt{
								From:   from,
								To:     admin.User.UserName,
								Amount: splitAmount,
								Reason: reason,
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(debt, "return"); err != nil {
								log.Printf("Error saving return: %v", err)
								continue
							}
							response.WriteString(fmt.Sprintf("%s вернул %s %.2f\n", from, admin.User.UserName, splitAmount))
						} else {
							// Split into two operations: return existing debt and create new debt
							// First, return the existing debt
							returnDebt := Debt{
								From:   from,
								To:     admin.User.UserName,
								Amount: returnAmount,
								Reason: reason + " (возврат)",
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(returnDebt, "return"); err != nil {
								log.Printf("Error saving return: %v", err)
								continue
							}

							// Then create new debt for the remaining amount
							newDebtAmount := splitAmount - returnAmount
							newDebt := Debt{
								From:   from,
								To:     admin.User.UserName,
								Amount: newDebtAmount,
								Reason: reason + " (новый долг)",
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(newDebt, "debt"); err != nil {
								log.Printf("Error saving new debt: %v", err)
								continue
							}
							response.WriteString(fmt.Sprintf("%s вернул %s %.2f и теперь %s должен %s %.2f\n",
								from, admin.User.UserName, returnAmount, admin.User.UserName, from, newDebtAmount))
						}
					} else {
						// Regular debt operation
						debt := Debt{
							From:   from,
							To:     admin.User.UserName,
							Amount: splitAmount,
							Reason: reason,
							ChatID: update.Message.Chat.ID,
							Time:   time.Now(),
						}
						if err := saveDebtWithType(debt, "debt"); err != nil {
							log.Printf("Error saving debt: %v", err)
							continue
						}
						response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", admin.User.UserName, from, splitAmount))
					}
				}
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, response.String())
			bot.Send(msg)
			continue
		}

		// Handle multiple users
		multiRe := regexp.MustCompile(`((?:@\w+\s+)+)(\d+(?:\.\d+)?)(?:\s+(.+))?`)
		multiMatches := multiRe.FindStringSubmatch(text)

		if multiMatches != nil {
			// Extract usernames
			usernames := regexp.MustCompile(`@(\w+)`).FindAllStringSubmatch(multiMatches[1], -1)
			if len(usernames) == 0 {
				continue
			}

			amount, _ := strconv.ParseFloat(multiMatches[2], 64)
			reason := ""
			if len(multiMatches) > 3 {
				reason = multiMatches[3]
			}

			// Split amount between users
			splitAmount := amount / float64(len(usernames))
			from := update.Message.From.UserName

			var response strings.Builder
			response.WriteString(fmt.Sprintf("Разделено %.2f между %d пользователями (по %.2f каждый):\n", amount, len(usernames), splitAmount))

			// Create debts for each user
			for _, username := range usernames {
				if username[1] != from {
					// Determine operation type and handle returns
					netBalance, err := getNetBalance(update.Message.Chat.ID, from, username[1])
					if err != nil {
						log.Printf("Error getting net balance: %v", err)
						continue
					}

					if netBalance < 0 {
						// This is a return operation
						returnAmount := -netBalance // Convert negative balance to positive amount
						if splitAmount <= returnAmount {
							// Simple return - amount is less than or equal to existing debt
							debt := Debt{
								From:   from,
								To:     username[1],
								Amount: splitAmount,
								Reason: reason,
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(debt, "return"); err != nil {
								log.Printf("Error saving return: %v", err)
								continue
							}
							response.WriteString(fmt.Sprintf("%s вернул(а) %s %.2f\n", from, username[1], splitAmount))
						} else {
							// Split into two operations: return existing debt and create new debt
							// First, return the existing debt
							returnDebt := Debt{
								From:   from,
								To:     username[1],
								Amount: returnAmount,
								Reason: reason + " (возврат)",
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(returnDebt, "return"); err != nil {
								log.Printf("Error saving return: %v", err)
								continue
							}

							// Then create new debt for the remaining amount
							newDebtAmount := splitAmount - returnAmount
							newDebt := Debt{
								From:   from,
								To:     username[1],
								Amount: newDebtAmount,
								Reason: reason + " (новый долг)",
								ChatID: update.Message.Chat.ID,
								Time:   time.Now(),
							}
							if err := saveDebtWithType(newDebt, "debt"); err != nil {
								log.Printf("Error saving new debt: %v", err)
								continue
							}
							response.WriteString(fmt.Sprintf("%s вернул(а) %s %.2f и теперь %s должен %s %.2f\n",
								from, username[1], returnAmount, username[1], from, newDebtAmount))
						}
					} else {
						// Regular debt operation
						debt := Debt{
							From:   from,
							To:     username[1],
							Amount: splitAmount,
							Reason: reason,
							ChatID: update.Message.Chat.ID,
							Time:   time.Now(),
						}
						if err := saveDebtWithType(debt, "debt"); err != nil {
							log.Printf("Error saving debt: %v", err)
							continue
						}
						response.WriteString(fmt.Sprintf("%s должен %s %.2f\n", username[1], from, splitAmount))
					}
				}
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, response.String())
			bot.Send(msg)
			continue
		}
	}
}

// getDebtHistory returns all debts for a specific chat within the last n days
func getDebtHistory(chatID int64, days int) ([]Debt, error) {
	rows, err := db.Query(`
		SELECT from_user, to_user, amount, reason, chat_id, created_at, operation_type
		FROM debts
		WHERE chat_id = ? AND datetime(created_at) >= datetime('now', ?)
		ORDER BY created_at DESC
	`, chatID, fmt.Sprintf("-%d days", days))
	if err != nil {
		log.Printf("Error querying debt history: %v", err)
		return nil, err
	}
	defer rows.Close()

	var debts []Debt
	for rows.Next() {
		var debt Debt
		var createdAt string
		var operationType string
		err := rows.Scan(&debt.From, &debt.To, &debt.Amount, &debt.Reason, &debt.ChatID, &createdAt, &operationType)
		if err != nil {
			log.Printf("Error scanning debt row: %v", err)
			return nil, err
		}
		debt.Time, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			log.Printf("Error parsing time '%s': %v", createdAt, err)
			return nil, err
		}
		debts = append(debts, debt)
	}
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating debt rows: %v", err)
		return nil, err
	}
	return debts, nil
}

// getChatDebts returns all debts for a specific chat
func getChatDebts(chatID int64) []Debt {
	rows, err := db.Query(`
		SELECT from_user, to_user, amount, reason, chat_id, created_at
		FROM debts
		WHERE chat_id = ?
		ORDER BY created_at DESC
	`, chatID)
	if err != nil {
		log.Printf("Error querying chat debts: %v", err)
		return nil
	}
	defer rows.Close()

	var debts []Debt
	for rows.Next() {
		var debt Debt
		var createdAt string
		err := rows.Scan(&debt.From, &debt.To, &debt.Amount, &debt.Reason, &debt.ChatID, &createdAt)
		if err != nil {
			log.Printf("Error scanning debt row: %v", err)
			continue
		}
		debt.Time, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			log.Printf("Error parsing time '%s': %v", createdAt, err)
			continue
		}
		debts = append(debts, debt)
	}
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating debt rows: %v", err)
		return nil
	}
	return debts
}

// Helper to get net balance between two users in a chat
func getNetBalance(chatID int64, userA, userB string) (float64, error) {
	var sumAtoB, sumBtoA float64
	err := db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM debts WHERE chat_id = ? AND from_user = ? AND to_user = ?`, chatID, userA, userB).Scan(&sumAtoB)
	if err != nil {
		return 0, err
	}
	err = db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM debts WHERE chat_id = ? AND from_user = ? AND to_user = ?`, chatID, userB, userA).Scan(&sumBtoA)
	if err != nil {
		return 0, err
	}
	return sumAtoB - sumBtoA, nil
}

// saveDebt saves a debt to the database
func saveDebt(debt Debt) error {
	_, err := db.Exec(`
		INSERT INTO debts (from_user, to_user, amount, reason, chat_id, created_at)
		VALUES (?, ?, ?, ?, ?, datetime(?))
	`, debt.From, debt.To, debt.Amount, debt.Reason, debt.ChatID, debt.Time.Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Printf("Error saving debt: %v", err)
		return err
	}
	return nil
}

// Add a new saveDebtWithType function:
func saveDebtWithType(debt Debt, opType string) error {
	_, err := db.Exec(`
		INSERT INTO debts (from_user, to_user, amount, reason, chat_id, created_at, operation_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, debt.From, debt.To, debt.Amount, debt.Reason, debt.ChatID, debt.Time.Format("2006-01-02 15:04:05"), opType)
	return err
} 