package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
)

const (
	SMTPHost     = "smtp.yandex.ru"
	SMTPPort     = "587"
	SMTPUsername = "79140050089@yandex.ru"
	SMTPPassword = "qoskpuzbchyqught"
	ToEmail      = "79140050089@yandex.ru"
	UploadDir    = "video"
	DBConnection = "myuser:mypassword@tcp(localhost:3306)/myapp?charset=utf8mb4"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func getUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		http.Error(w, "Не удалось подключиться к MySQL: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := db.Query("SELECT id, name FROM users")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Неверный JSON", http.StatusBadRequest)
		return
	}

	if input.Name == "" {
		http.Error(w, "Поле 'name' обязательно", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	result, err := db.Exec("INSERT INTO users (name) VALUES (?)", input.Name)
	if err != nil {
		http.Error(w, "Не удалось создать пользователя: "+err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(User{ID: int(id), Name: input.Name})
}

func initDB() {
	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Проверяем подключение
	err = db.Ping()
	if err != nil {
		log.Fatal("Не удалось подключиться к БД:", err)
	}

	// Создаем таблицу если не существует
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id INT AUTO_INCREMENT PRIMARY KEY,
            name VARCHAR(255) NOT NULL
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
	if err != nil {
		log.Fatal("Не удалось создать таблицу:", err)
	}

	// Добавляем тестовые данные если таблица пустая
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count == 0 {
		_, err = db.Exec(`INSERT INTO users (name) VALUES ('Алексей'), ('Мария')`)
		if err != nil {
			log.Println("Не удалось вставить тестовые данные:", err)
		} else {
			log.Println("Тестовые данные добавлены в MySQL.")
		}
	}

	log.Println("✅ База данных инициализирована успешно")
}

func main() {
	if err := os.MkdirAll(UploadDir, 0755); err != nil {
		log.Printf("Ошибка создания папки %s: %v", UploadDir, err)
	}
	initDB()

	go startDailyEmailScheduler()

	// API-эндпоинты
	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getUsers(w, r)
		case http.MethodPost:
			createUser(w, r)
		default:
			http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/upload-csv", uploadCSV)
	http.HandleFunc("/api/export-csv", exportCSV)
	http.HandleFunc("/api/send-csv-email", sendCSVHandler)

	// Новые эндпоинты для работы с видео
	http.HandleFunc("/api/upload-video", uploadVideoHandler)
	http.HandleFunc("/api/videos", listVideosHandler)
	http.HandleFunc("/api/video/", serveVideoHandler)
	http.HandleFunc("/api/delete-video/", deleteVideoHandler)

	// Раздача статики Angular из правильной папки
	staticDir := "../frontend/dist/browser"
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Проверяем API запросы - они не должны обрабатываться как статика
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		filePath := filepath.Join(staticDir, r.URL.Path)

		// Проверяем существование файла
		if _, err := os.Stat(filePath); err == nil && r.URL.Path != "/" {
			// Файл существует и это не корневой путь - отдаём его
			http.ServeFile(w, r, filePath)
		} else {
			// Файл не найден или корневой путь - отдаём index.html
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		}
	})

	log.Println("Сервер запущен на :8080")
	log.Println("Статика загружается из:", staticDir)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// uploadCSV обрабатывает загрузку CSV-файла
func uploadCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(100 << 20) // 100 MB
	if err != nil {
		http.Error(w, "Слишком большой файл или ошибка загрузки", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Файл не загружен. Используйте поле 'file'", http.StatusBadRequest)
		return
	}
	defer file.Close()

	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		http.Error(w, "Ошибка подключения к БД", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Не удалось начать транзакцию", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Очищаем таблицу
	_, err = tx.Exec("DELETE FROM users")
	if err != nil {
		http.Error(w, "Не удалось очистить таблицу", http.StatusInternalServerError)
		return
	}

	// 2. Читаем CSV
	reader := csv.NewReader(file)
	reader.Comma = ';' // разделитель — точка с запятой
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		http.Error(w, "Ошибка чтения CSV: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(records) == 0 {
		http.Error(w, "Файл пуст", http.StatusBadRequest)
		return
	}

	// 3. Находим начало данных (пропускаем мусор)
	startIndex := 0
	for i, record := range records {
		if len(record) >= 2 {
			col1 := strings.TrimSpace(record[0])
			col2 := strings.TrimSpace(record[1])
			if (col1 == "id" || col1 == "ID") && (col2 == "name" || col2 == "Name") {
				startIndex = i + 1
				break
			}
		}
	}
	// Если заголовок не найден — считаем, что данные с первой строки
	if startIndex == 0 && !(len(records) > 0 && len(records[0]) >= 2 && isNumeric(records[0][0])) {
		// Но если первая строка похожа на данные — оставляем startIndex = 0
		// Иначе можно оставить как есть
	}

	// 4. Вставляем данные
	inserted := 0
	for i := startIndex; i < len(records); i++ {
		record := records[i]
		if len(record) < 2 {
			continue
		}

		idStr := strings.TrimSpace(record[0])
		name := strings.TrimSpace(record[1])

		if idStr == "" || name == "" {
			continue
		}

		// Проверяем, что ID — число
		if !isNumeric(idStr) {
			continue // или выдать ошибку
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Неверный ID в строке %d: %s", i+1, idStr), http.StatusBadRequest)
			return
		}

		if !utf8.ValidString(name) {
			http.Error(w, fmt.Sprintf("Неверная кодировка в строке %d", i+1), http.StatusBadRequest)
			return
		}

		_, err = tx.Exec("INSERT INTO users (id, name) VALUES (?, ?)", id, name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Ошибка вставки в строке %d: %s", i+1, err.Error()), http.StatusInternalServerError)
			return
		}
		inserted++
	}

	// 5. Сохраняем
	err = tx.Commit()
	if err != nil {
		http.Error(w, "Ошибка сохранения данных", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Таблица users полностью заменена данными из CSV",
		"rows":    inserted,
	})
}

// Вспомогательная функция: проверяет, состоит ли строка из цифр (и, возможно, знака)
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}
func exportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Только GET", http.StatusMethodNotAllowed)
		return
	}

	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, name FROM users ORDER BY id")
	if err != nil {
		http.Error(w, "Query error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="users.csv"`)

	// BOM для Excel
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	writer.Comma = ';'                   // тот же разделитель!
	writer.Write([]string{"id", "name"}) // заголовок

	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		writer.Write([]string{fmt.Sprintf("%d", id), name})
	}
	writer.Flush()
}

// generateCSV создает CSV файл в памяти
func generateCSV() (*bytes.Buffer, error) {
	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, name FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM для UTF-8

	writer := csv.NewWriter(&buf)
	writer.Comma = ';'
	writer.Write([]string{"id", "name"})

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		writer.Write([]string{fmt.Sprintf("%d", id), name})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return &buf, nil
}

// sendCSVByEmail отправляет CSV файл по почте
func sendCSVByEmail() error {
	// Генерируем CSV
	csvData, err := generateCSV()
	if err != nil {
		return fmt.Errorf("ошибка генерации CSV: %v", err)
	}

	// Подсчитываем количество пользователей для темы письма
	db, err := sql.Open("mysql", DBConnection)
	if err != nil {
		return err
	}
	defer db.Close()

	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		userCount = 0
	}

	// Формируем тему письма с русской датой
	currentDate := time.Now().Format("02.01.2006") // формат DD.MM.YYYY
	subject := fmt.Sprintf("Бэкап от %s (%d записей)", currentDate, userCount)

	// Кодируем тему в MIME format для поддержки кириллицы
	encodedSubject := mime.QEncoding.Encode("UTF-8", subject)

	body := fmt.Sprintf("Во вложении CSV файл с пользователями.\n\nСгенерировано: %s\nКоличество записей: %d",
		time.Now().Format("2006-01-02 15:04:05"), userCount)

	// Создаем MIME сообщение
	boundary := "boundary12345"
	var msg bytes.Buffer

	// Заголовки
	msg.WriteString(fmt.Sprintf("From: %s\r\n", SMTPUsername))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", ToEmail))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", encodedSubject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n", boundary))
	msg.WriteString("\r\n")

	// Текст письма
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body + "\r\n")

	// Вложение
	filename := fmt.Sprintf("users_export_%s.csv", time.Now().Format("20060102"))
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/csv; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", filename))
	msg.WriteString("\r\n")

	// Кодируем CSV в base64
	encoded := make([]byte, base64.StdEncoding.EncodedLen(csvData.Len()))
	base64.StdEncoding.Encode(encoded, csvData.Bytes())

	// Пишем base64 построчно (требование RFC)
	lineLength := 76
	for i := 0; i < len(encoded); i += lineLength {
		end := i + lineLength
		if end > len(encoded) {
			end = len(encoded)
		}
		msg.Write(encoded[i:end])
		msg.WriteString("\r\n")
	}

	msg.WriteString("\r\n")
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	// Отправка с таймаутом
	auth := smtp.PlainAuth("", SMTPUsername, SMTPPassword, SMTPHost)

	// Создаем канал для таймаута
	done := make(chan error, 1)

	go func() {
		err = smtp.SendMail(SMTPHost+":"+SMTPPort, auth, SMTPUsername, []string{ToEmail}, msg.Bytes())
		done <- err
	}()

	// Ждем с таймаутом 30 секунд
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("ошибка отправки почты: %v", err)
		}
	case <-time.After(30 * time.Second):
		return fmt.Errorf("таймаут отправки почты: соединение заняло более 30 секунд")
	}

	log.Printf("Письмо с бэкапом отправлено: %s", subject)
	return nil
}

// sendCSVHandler обрабатывает запрос на отправку CSV по почте
func sendCSVHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	err := sendCSVByEmail()
	if err != nil {
		log.Printf("Ошибка отправки CSV: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "CSV файл успешно отправлен на почту",
	})
}

// startDailyEmailScheduler запускает планировщик ежедневной отправки
func startDailyEmailScheduler() {
	// Вычисляем время до следующего 09:00
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	durationUntilNext := next.Sub(now)

	// Запускаем первый раз через вычисленное время
	time.AfterFunc(durationUntilNext, func() {
		sendDailyEmail()
		// Затем каждые 24 часа
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			sendDailyEmail()
		}
	})

	log.Printf("Ежедневная отправка CSV настроена. Первая отправка в %s", next.Format("2006-01-02 15:04:05"))
}

// sendDailyEmail отправляет ежедневный отчет
func sendDailyEmail() {
	err := sendCSVByEmail()
	if err != nil {
		log.Printf("Ошибка ежедневной отправки CSV: %v", err)
	} else {
		log.Printf("Ежедневный CSV отправлен на почту: %s", time.Now().Format("2006-01-02 15:04:05"))
	}
}

// listVideosHandler возвращает список загруженных видео
func listVideosHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	// АБСОЛЮТНЫЙ путь к папке video
	videoDir := "/var/www/your-app/video"

	files, err := os.ReadDir(videoDir)
	if err != nil {
		log.Printf("Ошибка чтения папки %s: %v", videoDir, err)
		http.Error(w, "Ошибка чтения папки с видео", http.StatusInternalServerError)
		return
	}

	var videos []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() && isVideoFile(file.Name()) {
			info, err := file.Info()
			if err != nil {
				continue
			}

			videos = append(videos, map[string]interface{}{
				"filename":    file.Name(),
				"size":        info.Size(),
				"uploaded_at": info.ModTime().Format("2006-01-02 15:04:05"),
				"url":         "/video/" + file.Name(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(videos)
}

// serveVideoHandler отдает видео файл
func serveVideoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/video/")
	if filename == "" {
		http.Error(w, "Не указано имя файла", http.StatusBadRequest)
		return
	}

	// Защита от path traversal атак
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "Некорректное имя файла", http.StatusBadRequest)
		return
	}

	// АБСОЛЮТНЫЙ путь к файлу
	filePath := filepath.Join("/var/www/your-app/video", filename)

	// Проверяем существование файла
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log.Printf("Файл не найден: %s", filePath)
		http.Error(w, "Файл не найден", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Ошибка доступа к файлу %s: %v", filePath, err)
		http.Error(w, "Ошибка доступа к файлу", http.StatusInternalServerError)
		return
	}

	// Проверяем, что это файл, а не директория
	if fileInfo.IsDir() {
		http.Error(w, "Указанный путь является директорией", http.StatusBadRequest)
		return
	}

	// Проверяем, что это видео файл
	if !isVideoFile(filename) {
		http.Error(w, "Файл не является видео", http.StatusBadRequest)
		return
	}

	// Открываем файл для чтения
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Не удалось открыть файл %s: %v", filePath, err)
		http.Error(w, "Не удалось открыть файл", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Устанавливаем правильный Content-Type
	ext := strings.ToLower(filepath.Ext(filename))
	contentType := getContentType(ext)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "video/mp4") // fallback
	}

	// Устанавливаем заголовки для правильной работы в браузере
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// Используем ServeContent вместо ServeFile для лучшей поддержки стриминга
	http.ServeContent(w, r, filename, fileInfo.ModTime(), file)
}

// uploadVideoHandler обрабатывает загрузку видео
func uploadVideoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	// Ограничиваем размер файла (100MB)
	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, "Слишком большой файл (макс. 100MB) или ошибка загрузки", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "Файл не загружен. Используйте поле 'video'", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Проверяем тип файла
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		http.Error(w, "Ошибка чтения файла: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Проверяем, что это видео файл
	contentType := http.DetectContentType(buffer)
	if !strings.HasPrefix(contentType, "video/") {
		http.Error(w, fmt.Sprintf("Файл должен быть видео. Обнаружен тип: %s", contentType), http.StatusBadRequest)
		return
	}

	// Проверяем расширение файла
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isVideoFile(header.Filename) {
		http.Error(w, fmt.Sprintf("Неподдерживаемый формат видео: %s", ext), http.StatusBadRequest)
		return
	}

	// Возвращаемся к началу файла
	_, err = file.Seek(0, 0)
	if err != nil {
		http.Error(w, "Ошибка чтения файла: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Создаем уникальное имя файла
	filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), generateRandomString(8), ext)
	filePath := filepath.Join("/var/www/your-app/video", filename)

	// Создаем файл на сервере
	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Не удалось сохранить файл: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Копируем содержимое файла
	bytesCopied, err := io.Copy(dst, file)
	if err != nil {
		// Удаляем частично загруженный файл в случае ошибки
		os.Remove(filePath)
		http.Error(w, "Ошибка сохранения файла: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Получаем информацию о файле
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "Ошибка получения информации о файле: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Видео успешно загружено: %s (%d bytes)", filename, bytesCopied)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "success",
		"message":     "Видео успешно загружено",
		"filename":    filename,
		"size":        fileInfo.Size(),
		"uploaded_at": time.Now().Format("2006-01-02 15:04:05"),
		"url":         "/video/" + filename,
	})
}

// deleteVideoHandler удаляет видео файл
func deleteVideoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/api/delete-video/")
	if filename == "" {
		http.Error(w, "Не указано имя файла", http.StatusBadRequest)
		return
	}

	// Защита от path traversal атак
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "Некорректное имя файла", http.StatusBadRequest)
		return
	}

	// АБСОЛЮТНЫЙ путь к файлу
	filePath := filepath.Join("/var/www/your-app/video", filename)

	// Проверяем существование файла
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log.Printf("Файл не найден для удаления: %s", filePath)
		http.Error(w, "Файл не найден", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Ошибка доступа к файлу %s: %v", filePath, err)
		http.Error(w, "Ошибка доступа к файлу", http.StatusInternalServerError)
		return
	}

	// Проверяем, что это файл, а не директория
	if fileInfo.IsDir() {
		http.Error(w, "Указанный путь является директорией", http.StatusBadRequest)
		return
	}

	// Проверяем, что это видео файл
	if !isVideoFile(filename) {
		http.Error(w, "Файл не является видео", http.StatusBadRequest)
		return
	}

	// Удаляем файл
	err = os.Remove(filePath)
	if err != nil {
		log.Printf("Ошибка удаления файла %s: %v", filePath, err)
		http.Error(w, "Ошибка удаления файла", http.StatusInternalServerError)
		return
	}

	log.Printf("Видео удалено: %s", filename)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Видео успешно удалено",
	})
}

// Вспомогательные функции

// generateRandomString генерирует случайную строку
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// isVideoFile проверяет, является ли файл видео
func isVideoFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	videoExtensions := map[string]bool{
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
		".mkv":  true,
		".m4v":  true,
		".3gp":  true,
	}
	return videoExtensions[ext]
}

// getContentType возвращает Content-Type для расширения файла
func getContentType(ext string) string {
	contentTypes := map[string]string{
		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".mkv":  "video/x-matroska",
		".m4v":  "video/x-m4v",
		".3gp":  "video/3gpp",
	}
	return contentTypes[ext]
}
