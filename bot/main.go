package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	log.Println("Запуск Телеграм-бота...")

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	adminIDStr := os.Getenv("ADMIN_TG_ID")
	masterURL := os.Getenv("MASTER_URL")
	botSecret := os.Getenv("BOT_SECRET")

	if botToken == "" || adminIDStr == "" || masterURL == "" || botSecret == "" {
		log.Fatal("Критическая ошибка: не все переменные окружения заданы (TELEGRAM_BOT_TOKEN, ADMIN_TG_ID, MASTER_URL, BOT_SECRET).")
	}

	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		log.Fatal("Невалидный ADMIN_TG_ID")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Ошибка инициализации Telegram API: %v", err)
	}

	core := NewCoreClient(masterURL, botSecret)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// Счетчик для имитации ID заявок в рамках MVP (в боевой системе пишется в payment_requests)
	paymentCounter := 1000

	for update := range updates {
		// 1. Обработка нажатий инлайн-кнопок админом
		if update.CallbackQuery != nil {
			handleCallback(bot, core, update.CallbackQuery, adminID)
			continue
		}

		if update.Message == nil {
			continue
		}

		msg := update.Message
		chatID := msg.Chat.ID

		// 2. Обработка текстовых команд
		switch msg.Text {
		case "/start":
			// По ТЗ при старте создаем пользователя со статусом free
			err := core.CreateUser(chatID, "free")
			if err != nil {
				log.Printf("Ошибка авторегистрации юзера %d: %v", chatID, err)
				bot.Send(tgbotapi.NewMessage(chatID, "Ошибка регистрации в системе."))
				continue
			}

			reply := "Добро пожаловать в GradVPN!\nВаш аккаунт зарегистрирован на бесплатном (замедленном) тарифе.\n\nИспользуйте /info для получения ключей."
			bot.Send(tgbotapi.NewMessage(chatID, reply))

		case "/info":
			user, err := core.GetUser(chatID)
			if err != nil || user == nil {
				bot.Send(tgbotapi.NewMessage(chatID, "Пользователь не найден. Введите /start"))
				continue
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Ваш тариф: %s\n", strings.ToUpper(user.Tier)))
			if user.ExpireAt != nil {
				sb.WriteString(fmt.Sprintf("Активен до: %s\n", *user.ExpireAt))
			} else if user.Tier == "free" {
				sb.WriteString("⚠️ Трафик искусственно замедлен ядрами Linux (tc троттлинг). Оплатите Premium для высокой скорости.\n")
			}

			sb.WriteString("\n Ваши ключи подключения:\n")
			if user.VlessUUID != nil {
				sb.WriteString(fmt.Sprintf("VLESS (Free/Premium):\n`vless://%s@%s:443?encryption=none&security=tls&type=httpupgrade`\n\n", *user.VlessUUID, "node_address"))
			}
			if user.Tier == "premium" && user.Hy2Password != nil {
				sb.WriteString(fmt.Sprintf("Hysteria 2 (Premium):\n`hy2://%s@%s:8443`\n", *user.Hy2Password, "node_address"))
			}

			msgOut := tgbotapi.NewMessage(chatID, sb.String())
			msgOut.ParseMode = "Markdown"
			bot.Send(msgOut)

		case "Я оплатил":
			paymentCounter++
			// Уведомление пользователю
			bot.Send(tgbotapi.NewMessage(chatID, "Заявка отправлена администратору на проверку. Ожидайте начисления подписки."))

			// Отправка админу карточки на подтверждение
			adminMsg := tgbotapi.NewMessage(adminID, fmt.Sprintf("User %d утверждает, что оплатил Premium.\nID транзакции: #%d", chatID, paymentCounter))
			
			inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Подтвердить", fmt.Sprintf("approve_%d_%d", paymentCounter, chatID)),
					tgbotapi.NewInlineKeyboardButtonData("Отклонить", fmt.Sprintf("reject_%d_%d", paymentCounter, chatID)),
				),
			)
			adminMsg.ReplyMarkup = inlineKeyboard
			bot.Send(adminMsg)

		default:
			bot.Send(tgbotapi.NewMessage(chatID, "Неизвестная команда. Доступны: /start, /info, или напишите 'Я оплатил' для отправки чека."))
		}
	}
}

func handleCallback(bot *tgbotapi.BotAPI, core *CoreClient, cb *tgbotapi.CallbackQuery, adminID int64) {
	if cb.From.ID != adminID {
		return // На кнопки может нажимать только админ
	}

	parts := strings.Split(cb.Data, "_")
	if len(parts) != 3 {
		return
	}

	action := parts[0]
	paymentID, _ := strconv.Atoi(parts[1])
	userTgID, _ := strconv.ParseInt(parts[2], 10, 64)

	var text string
	if action == "approve" {
		// Дергаем ядро для аппрува и автоматического начисления премиума
		err := core.ApprovePayment(paymentID)
		if err != nil {
			log.Printf("Ошибка аппрува платежа %d на ядре: %v", paymentID, err)
			return
		}

		// Запрос на ручной перевод юзера в премиум (запасной/явный апдейт)
		_ = core.CreateUser(userTgID, "premium")

		text = fmt.Sprintf("Заявка #%d подтверждена. Пользователю выдан Premium.", paymentID)
		bot.Send(tgbotapi.NewMessage(userTgID, "🎉 Ваша оплата подтверждена! Premium тариф успешно активирован. Проверьте новые ключи в /info."))
	} else {
		text = fmt.Sprintf("Заявка #%d отклонена.", paymentID)
		bot.Send(tgbotapi.NewMessage(userTgID, "❌ Администратор отклонил вашу заявку на оплату. Проверьте реквизиты перевода."))
	}

	// Обновляем статус сообщения у админа, чтобы убрать кнопки
	editMsg := tgbotapi.NewEditMessageText(adminID, cb.Message.MessageID, text)
	bot.Send(editMsg)
}
