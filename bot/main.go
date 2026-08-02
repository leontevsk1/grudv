package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const paymentReminderInterval = 24 * time.Hour

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

	go runPaymentReminderLoop(bot, core)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	// Telegram запоминает allowed_updates последнего вызова getUpdates и
	// продолжает фильтровать по нему даже после рестарта бота, пока не
	// передать список явно — с прошлого эксперимента здесь висел
	// фильтр ["message"], из-за которого callback_query (нажатия кнопок)
	// не долетали до бота вовсе.
	u.AllowedUpdates = []string{"message", "callback_query"}
	updates := bot.GetUpdatesChan(u)

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
			startMsg := tgbotapi.NewMessage(chatID, reply)
			startMsg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("Оплатить")),
			)
			bot.Send(startMsg)

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

			sb.WriteString(fmt.Sprintf("\nСсылка на подписку:\n`%s/api/sub/%d`\n", core.MasterURL, chatID))
			sb.WriteString("\nЭта ссылка работает со всеми современными VPN-клиентами (sing-box, xray и т.д.)")

			msgOut := tgbotapi.NewMessage(chatID, sb.String())
			msgOut.ParseMode = "Markdown"
			bot.Send(msgOut)

		case "Оплатить":
			sendPaymentCodeCard(bot, core, chatID)

		default:
			bot.Send(tgbotapi.NewMessage(chatID, "Неизвестная команда. Доступны: /start, /info, или напишите 'Оплатить' для получения кода платежа."))
		}
	}
}

// Код действует один календарный день — vpn-core сам решает, выдать существующий
// или сгенерировать новый (get_or_create_payment_code), поэтому повторное нажатие
// "Оплатить" в тот же день безопасно и просто повторно показывает тот же код.
func sendPaymentCodeCard(bot *tgbotapi.BotAPI, core *CoreClient, chatID int64) {
	code, err := core.GetPaymentCode(chatID)
	if err != nil {
		log.Printf("Ошибка получения кода оплаты для %d: %v", chatID, err)
		bot.Send(tgbotapi.NewMessage(chatID, "Не удалось получить код оплаты, попробуйте позже."))
		return
	}

	text := fmt.Sprintf("Для оплаты Premium переведите оплату администратору и укажите в комментарии к платежу код:\n\n`%s`\n\nПосле перевода нажмите «Подтвердить».", code)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Отмена", "pay_cancel_"+code),
			tgbotapi.NewInlineKeyboardButtonData("Подтвердить", "pay_confirm_"+code),
		),
	)
	bot.Send(msg)
}

func runPaymentReminderLoop(bot *tgbotapi.BotAPI, core *CoreClient) {
	ticker := time.NewTicker(paymentReminderInterval)
	defer ticker.Stop()

	for range ticker.C {
		users, err := core.GetFreeUsers()
		if err != nil {
			log.Printf("Ошибка получения списка free-пользователей для напоминания: %v", err)
			continue
		}

		for _, user := range users {
			msg := tgbotapi.NewMessage(user.TgID, "⚠️ Напоминаем: ваш тариф — бесплатный (замедленная скорость). Нажмите «Оплатить», чтобы перейти на Premium.")
			msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("Оплатить")),
			)
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Ошибка отправки напоминания об оплате пользователю %d: %v", user.TgID, err)
			}
		}
	}
}

func handleCallback(bot *tgbotapi.BotAPI, core *CoreClient, cb *tgbotapi.CallbackQuery, adminID int64) {
	bot.Request(tgbotapi.NewCallback(cb.ID, ""))

	if strings.HasPrefix(cb.Data, "pay_") {
		handlePaymentCallback(bot, core, cb, adminID)
		return
	}

	handleAdminCallback(bot, core, cb, adminID)
}

// Заявка в vpn-core создаётся только по нажатию "Подтвердить" — если пользователь
// жмёт "Отмена", никакого payment_request не появляется и админ ничего не видит.
func handlePaymentCallback(bot *tgbotapi.BotAPI, core *CoreClient, cb *tgbotapi.CallbackQuery, adminID int64) {
	chatID := cb.From.ID

	parts := strings.SplitN(cb.Data, "_", 3)
	if len(parts) != 3 {
		log.Printf("DEBUG payment callback: неверный формат data, parts=%v", parts)
		return
	}
	action := parts[1]
	code := parts[2]

	if action == "cancel" {
		bot.Send(tgbotapi.NewEditMessageText(chatID, cb.Message.MessageID, "Оплата отменена."))
		return
	}

	paymentID, err := core.CreatePaymentRequest(chatID, code)
	if err != nil {
		log.Printf("Ошибка создания заявки на оплату для %d: %v", chatID, err)
		bot.Send(tgbotapi.NewEditMessageText(chatID, cb.Message.MessageID, "Не удалось отправить заявку, попробуйте позже."))
		return
	}

	bot.Send(tgbotapi.NewEditMessageText(chatID, cb.Message.MessageID, "Заявка отправлена администратору на проверку. Ожидайте начисления подписки."))

	adminMsg := tgbotapi.NewMessage(adminID, fmt.Sprintf("User %d утверждает, что оплатил Premium.\nКод платежа: %s\nID заявки: #%d", chatID, code, paymentID))
	adminMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подтвердить", fmt.Sprintf("approve_%d_%d", paymentID, chatID)),
			tgbotapi.NewInlineKeyboardButtonData("Отклонить", fmt.Sprintf("reject_%d_%d", paymentID, chatID)),
		),
	)
	bot.Send(adminMsg)
}

func handleAdminCallback(bot *tgbotapi.BotAPI, core *CoreClient, cb *tgbotapi.CallbackQuery, adminID int64) {
	log.Printf("DEBUG callback: from=%d adminID=%d data=%q", cb.From.ID, adminID, cb.Data)

	if cb.From.ID != adminID {
		log.Printf("DEBUG callback: отклонён, from != adminID")
		return // На кнопки может нажимать только админ
	}

	parts := strings.Split(cb.Data, "_")
	if len(parts) != 3 {
		log.Printf("DEBUG callback: неверный формат data, parts=%v", parts)
		return
	}

	action := parts[0]
	paymentID, paymentErr := strconv.Atoi(parts[1])
	userTgID, userErr := strconv.ParseInt(parts[2], 10, 64)
	log.Printf("DEBUG callback: action=%s paymentID=%d (err=%v) userTgID=%d (err=%v)", action, paymentID, paymentErr, userTgID, userErr)

	var text string
	if action == "approve" {
		// Дергаем ядро для аппрува и автоматического начисления премиума
		err := core.ApprovePayment(paymentID)
		if err != nil {
			log.Printf("Ошибка аппрува платежа %d на ядре: %v", paymentID, err)
			return
		}
		log.Printf("DEBUG callback: ApprovePayment(%d) успешно", paymentID)

		// Запрос на ручной перевод юзера в премиум (запасной/явный апдейт)
		createErr := core.CreateUser(userTgID, "premium")
		log.Printf("DEBUG callback: CreateUser(%d, premium) err=%v", userTgID, createErr)

		text = fmt.Sprintf("Заявка #%d подтверждена. Пользователю выдан Premium.", paymentID)
		bot.Send(tgbotapi.NewMessage(userTgID, "🎉 Ваша оплата подтверждена! Premium тариф успешно активирован. Проверьте новые ключи в /info."))
	} else {
		if err := core.RejectPayment(paymentID); err != nil {
			log.Printf("Ошибка отклонения платежа %d на ядре: %v", paymentID, err)
		}
		text = fmt.Sprintf("Заявка #%d отклонена.", paymentID)
		bot.Send(tgbotapi.NewMessage(userTgID, "❌ Администратор отклонил вашу заявку на оплату. Проверьте реквизиты перевода."))
	}

	// Обновляем статус сообщения у админа, чтобы убрать кнопки
	editMsg := tgbotapi.NewEditMessageText(adminID, cb.Message.MessageID, text)
	bot.Send(editMsg)
}
