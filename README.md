# БипЧат / Messenger_GO

Go 1.25 + PostgreSQL (pgx/v5) + JWT + Gorilla WebSocket + адаптивный frontend из `web/`.
Redis используется только для online presence; Kafka в локальном режиме не запускается и не требуется для переписки.

## Быстрый запуск на Windows 11

1. Установите Go 1.25+ и Docker Desktop (с включённым Linux-контейнерным движком).
2. Откройте PowerShell/`cmd` **из корня репозитория** и выполните:

   ```bat
   start.bat
   ```

`start.bat` сам:

- проверит версию Go и наличие Docker Desktop;
- создаст `.env` из `.env.example`, если файла ещё нет (существующий `.env` не трогается);
- подставит случайный `JWT_SECRET` вместо плейсхолдера при первом создании `.env`;
- поднимет `postgres` и `redis` через `docker compose` (Kafka вынесена в профиль `kafka`);
- дождётся готовности PostgreSQL через `pg_isready`, а не фиксированным `timeout`;
- применит миграции через встроенный Go runner: `go run ./cmd/migrate -path ./migrations up`;
- запустит backend из корня репозитория.

После запуска откройте **http://localhost:8080**. Отдельный frontend-сервер не нужен: `web/` раздаётся тем же Go-процессом.

### Без Docker Desktop

Если Docker недоступен, используйте уже установленный PostgreSQL:

1. Создайте базу и пользователя, например:

   ```sql
   CREATE USER messenger WITH PASSWORD 'messenger_pass';
   CREATE DATABASE messenger_db OWNER messenger;
   ```

2. Создайте `.env` из `.env.example` и укажите `DB_SOURCE` с адресом вашего сервера
   (для локального PostgreSQL на хосте — `localhost:5432`).
3. Выполните `set USE_LOCAL_PG=1 && start.bat`.
4. При желании запустите Redis локально или оставьте presence без Redis — переписка работает и без него.

## Миграции

| Файл | Назначение |
| --- | --- |
| `001_create_users.up.sql` | пользователи (файл переименован из `001_create_users.sql` для совместимости с `migrate`; версия и описание сохранились) |
| `002…006` | чаты, сообщения, улучшения чатов, аватар, чат «Избранное» |
| `007_academic_groups.up.sql` | таблица `academic_groups` с ровно пятью значениями `Т-491`–`Т-495`, nullable `users.group_id` с FK, уникальный индекс для чата «Избранное» |
| `007_academic_groups.down.sql` | откат миграции 007 |

Старые пользователи получают `group_id = NULL` и продолжают работать. Новая миграция не удаляет users/chats/messages.

Применение вручную (один из способов):

```powershell
# Встроенный runner, не требует установленного CLI:
$env:DB_SOURCE = (Select-String -Path .env -Pattern '^DB_SOURCE=').Line.Split('=',2)[1]
go run ./cmd/migrate -path ./migrations up
go run ./cmd/migrate -path ./migrations status

#либо через CLI golang-migrate:
make migrate-up
```

Откат одной миграции: `go run ./cmd/migrate -path ./migrations down 1`.
Если миграция помечена `dirty`, исправьте причину и выполните `go run ./cmd/migrate -path ./migrations force <version>`.

## Учебные группы

- frontend отправляет `group_name` при регистрации и проверяет список из пяти значений;
- backend дополнительно проверяет значение по списку `domain.AcademicGroups` **и** по таблице `academic_groups`;
- `users.group_id` записывается тем же `INSERT`, что и пользователь (атомарно);
- `group_name` возвращается в `/auth/register`, `/users/me`, `/users/search`, `/users/{userID}`;
- `PUT /users/profile` не принимает `group_name` и не может изменить группу.

Раздел «Группы» в интерфейсе — это **групповые чаты**, а не учебная группа. Автоматического создания чата учебной группы нет.

## API (фактические контракты)

- `POST /api/v1/auth/register` — `username`, `name`, `email`, `password`, `group_name` → User;
- `POST /api/v1/auth/login` — поле `email` принимает **или email, или username** → `{token, user}`;
- `GET /api/v1/users/me`, `GET /api/v1/users/search?q=…`, `PUT /api/v1/users/profile`, `GET /api/v1/users/{userID}`;
- `GET /api/v1/chats/`, `POST /api/v1/chats/private`, `POST /api/v1/chats/group`, `GET /api/v1/chats/{chatID}`;
- `POST /api/v1/chats/{chatID}/members`, `DELETE /api/v1/chats/{chatID}/members/{userID}` (только owner/admin группового чата);
- `GET/POST /api/v1/chats/{chatID}/messages`, `POST /api/v1/chats/{chatID}/read`;
- `POST /api/v1/upload/avatar`, `GET /ws?token=…`, `GET /healthz`.

Список чатов возвращает `domain.ChatListItem`: `id`, `type`, `name`, `partner_id`, `last_message`,
`last_message_time`, `last_sender_id`, `unread_count`, `created_at`. Чтение, отправка и отметка прочтения
проверяют членство в чате. `limit` истории ограничен 100.

WebSocket отправляет события `{"type":"new_message","payload":{...}}`; frontend подключается к `/ws`,
но оставляет polling раз в 4 секунды как fallback. Сообщения дедуплицируются по `id`.
CORS и `CheckOrigin` разрешают same-origin и явно перечисленные адреса из `CORS_ALLOWED_ORIGINS`.

## Тесты

```powershell
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build -o .\bin\messenger.exe .\cmd\messenger
```

Часть тестов работает на in-memory реализациях хранилищ и не требует БД. Интеграционные тесты
в `internal/integration` запускаются только когда задан `TEST_DB_SOURCE` с адресом **тестовой** БД,
где уже применены миграции:

```powershell
$env:TEST_DB_SOURCE = 'postgres://messenger:messenger_pass@localhost:5432/messenger_test?sslmode=disable'
go run ./cmd/migrate -path ./migrations up   # DB_SOURCE должен указывать на ту же тестовую БД
go test ./internal/integration -v
```

Тесты не создают демо-аккаунты в рабочей базе: они очищают таблицы только в `messenger_test`.

## Ограничения

- Общего обмена файлами/вложениями в чатах нет: страница «Файлы» честно показывает пустое состояние.
  Реализована только загрузка аватара.
- JWT stateless: серверного отзыва токена нет, logout очищает только локальный токен и кеш пользователя.
- Kafka подключена опционально; при запуске из `main.go` writer не создаётся, поэтому брокер не нужен.
- Redis нужен только для online presence; при недоступности Redis переписка продолжает работать.
