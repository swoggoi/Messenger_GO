@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0"

echo === БипЧат: запуск на Windows ===

where go >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Go не найден в PATH. Установите Go 1.25 или новее.
  exit /b 1
)

for /f "tokens=3" %%v in ('go version') do set "GO_VERSION=%%v"
set "GO_VERSION=!GO_VERSION:go=!"
for /f "tokens=1,2 delims=." %%a in ("!GO_VERSION!") do (
  set "GO_MAJOR=%%a"
  set "GO_MINOR=%%b"
)
if not "!GO_MAJOR!"=="1" (
  if !GO_MAJOR! LSS 2 (
    echo [ERROR] Нужна версия Go 1.25 или новее, найдена !GO_VERSION!.
    exit /b 1
  )
) else (
  if !GO_MINOR! LSS 25 (
    echo [ERROR] Нужна версия Go 1.25 или новее, найдена !GO_VERSION!.
    exit /b 1
  )
)
echo Go: !GO_VERSION!

if not defined USE_LOCAL_PG (
  docker info >nul 2>&1
  if errorlevel 1 (
    echo [ERROR] Docker Desktop не запущен. Запустите Docker Desktop или
    echo         установите USE_LOCAL_PG=1 и настройте DB_SOURCE на свой PostgreSQL
    echo         (подробности в README.md).
    exit /b 1
  )
  echo Docker Desktop: OK
)

if not exist ".env" (
  copy /Y ".env.example" ".env" >nul
  echo Создан .env из .env.example (существующие файлы не перезаписываются).
)

for /f "usebackq delims=" %%S in (`powershell -NoProfile -Command "$b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)"`) do set "NEW_SECRET=%%S"
powershell -NoProfile -Command "$c = [IO.File]::ReadAllText('.env'); if ($c -match '(?m)^JWT_SECRET=REPLACE_WITH') { $c = [regex]::Replace($c, '(?m)^JWT_SECRET=.*$', 'JWT_SECRET=' + $env:NEW_SECRET); [IO.File]::WriteAllText('.env', $c); Write-Output 'JWT_SECRET сгенерирован и сохранён в .env' }"

if not defined USE_LOCAL_PG (
  echo Запуск PostgreSQL и Redis через Docker Compose...
  docker compose up -d postgres redis
  if errorlevel 1 (
    echo [ERROR] Не удалось запустить инфраструктуру через Docker Compose.
    exit /b 1
  )
  echo Ожидание готовности PostgreSQL...
  set "READY=0"
  for /L %%i in (1,1,60) do (
    if "!READY!"=="0" (
      docker compose exec -T postgres pg_isready -U messenger -d messenger_db >nul 2>&1
      if not errorlevel 1 set "READY=1"
      if "!READY!"=="0" timeout /t 1 /nobreak >nul
    )
  )
  if "!READY!"=="0" (
    echo [ERROR] PostgreSQL не стал доступен за 60 секунд.
    exit /b 1
  )
) else (
  echo Проверка локального PostgreSQL (порт из DB_SOURCE)...
  powershell -NoProfile -Command "$c = [IO.File]::ReadAllText('.env'); $m = [regex]::Match($c, '(?m)^DB_SOURCE=.*?@([^:/]+):(\d+)'); if (-not $m.Success) { exit 1 }; $hostName = $m.Groups[1].Value; $port = [int]$m.Groups[2].Value; for ($i = 0; $i -lt 60; $i++) { $client = New-Object Net.Sockets.TcpClient; try { $client.Connect($hostName, $port); $client.Close(); exit 0 } catch { $client.Close(); Start-Sleep -Seconds 1 } }; exit 1"
  if errorlevel 1 (
    echo [ERROR] Локальный PostgreSQL недоступен. Проверьте DB_SOURCE в .env.
    exit /b 1
  )
)
echo PostgreSQL готов.

echo Применение миграций...
go run ./cmd/migrate -path ./migrations up
if errorlevel 1 (
  echo [ERROR] Миграции не применены. Запуск остановлен.
  exit /b 1
)

echo.
echo БипЧат будет доступен по адресу: http://localhost:8080
echo Для остановки нажмите Ctrl+C.
echo.
go run ./cmd/messenger
if errorlevel 1 (
  echo [ERROR] Backend завершился с ошибкой (код !errorlevel!).
  exit /b 1
)
endlocal
