# go-musthave-shortener-tpl

Шаблон репозитория для трека «Сервис сокращения URL».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-shortener-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Инкремент 17. Бенчмарки и профилирование памяти

Добавлены бенчмарки для `POST /`, `POST /api/shorten`, batch из 32 URL, `GET /{id}`, gzip-ответа, а также `Memory.Save` и `Memory.Find`. HTTP-сценарии используют `httptest` и memory storage, без PostgreSQL и внешних сервисов. Полные реальные выводы до и после находятся в `profiles/benchmark-before.txt` и `profiles/benchmark-after.txt`.

Замеры выполнены на Go 1.26.1 (`darwin/arm64`, Apple M1 Max). Для обоих наборов применялась команда:

```bash
env GOCACHE=/private/tmp/shortener-gocache go test ./internal/handler ./internal/repository -run '^$' -bench . -benchmem -count=3
```

Исходные бенчмарки и профиль сняты в отдельном временном worktree на точном коммите `4826d47`; туда добавлялись только те же benchmark-файлы. Профиль до и после построен на 2 000 операциях командой:

```bash
env GOCACHE=/private/tmp/shortener-gocache go test ./internal/repository -run '^$' -bench '^BenchmarkMemorySave$' -benchtime=2000x -count=1 -memprofile="$(pwd)/profiles/base.pprof" -benchmem
```

Для повторного прогона использовалась та же команда с `-memprofile="$(pwd)/profiles/result.pprof"`.

До изменения каждый вызов `Memory.Save` копировал весь список записей в snapshot, хотя memory-only storage не пишет его на диск. В `alloc_space` главным источником этих временных выделений оказался `cloneRecords`. В `inuse_space` существенного уменьшения удерживаемой памяти приложения нет: оптимизация убирает временные копии при сохранении, а не данные, которые repository хранит.

Теперь `Memory.Save` клонирует snapshot только при заданном file path. Запись файла, блокировки и обработка ошибок файлового backend сохранены.

Для ранее проведённого измерения `MemorySave` на 10 000 операций медианы были: до — 107413 ns/op, 365276 B/op, 30 allocs/op; после — 2327 ns/op, 1188 B/op, 29 allocs/op. Свежий сохранённый прогон `-count=3` даёт отдельные медианы: до — 108782 ns/op, 365272 B/op, 30 allocs/op; после — 2301 ns/op, 1173 B/op, 29 allocs/op. В фиксированном профильном запуске на 2 000 операций результаты составили 18193 ns/op и 76571 B/op до оптимизации и 2316 ns/op и 1061 B/op после; число аллокаций снизилось с 29 до 28. Эти разные прогоны не объединяются в одну метрику.

Профили `profiles/base.pprof` и `profiles/result.pprof` проверены утилитой pprof. В Go SDK в окружении нет команды `go tool pprof`, поэтому использована отдельно установленная утилита `pprof`; ниже реальный вывод команды `pprof -top -diff_base=profiles/base.pprof profiles/result.pprof`. Полные результаты для `alloc_space` и `inuse_space` сохранены в `profiles/pprof-diff.txt`.

```text
File: repository.test
Type: alloc_space
Time: 2026-11-03 00:45:56 MSK
Showing nodes accounting for -141.36MB, 97.14% of 145.52MB total
Dropped 20 nodes (cum <= 0.73MB)
      flat  flat%   sum%        cum   cum%
 -138.86MB 95.42% 95.42%  -138.86MB 95.42%  github.com/true18/shortener/internal/repository.cloneRecords (inline)
   -1.50MB  1.03% 96.45%  -140.88MB 96.81%  github.com/true18/shortener/internal/repository.BenchmarkMemorySave
      -1MB  0.69% 97.14%       -1MB  0.69%  crypto/rand.Int
         0     0% 97.14%  -138.88MB 95.43%  github.com/true18/shortener/internal/repository.(*Memory).Save
         0     0% 97.14%       -1MB  0.69%  github.com/true18/shortener/internal/repository.(*Memory).newIDLocked
         0     0% 97.14%       -1MB  0.69%  github.com/true18/shortener/internal/repository.newID
         0     0% 97.14%  -140.88MB 96.81%  testing.(*B).launch
         0     0% 97.14%  -140.88MB 96.81%  testing.(*B).runN
```

Diff по суммарным выделениям показывает −141.36 MB, в том числе −138.86 MB в `cloneRecords`. По удерживаемой памяти разница только −2.02 kB (около 0.1% профиля) и относится преимущественно к runtime. Это подтверждает уменьшение временных аллокаций конкретной операции, но не снижение общего потребления памяти всего сервиса.

В дополнительном review-замере проверены no-op вызовы на хранилище с 4096 записями. Повторный `SaveBatch` из 32 уже сохранённых URL снизился с медианы 91575 ns/op и 304256 B/op до 712 ns/op и 1152 B/op; аллокации снизились с 2 до 1 на операцию. `DeleteUserURLs` для этих записей от другого пользователя снизился с 207673 ns/op и 578882 B/op до 115127 ns/op и 283968 B/op; аллокации — с 19 до 18. В `BenchmarkCreateBatch` (32 URL) медиана изменилась с 42622 ns/op, 31806 B/op и 164 allocs/op до 42262 ns/op, 29105 B/op и 163 allocs/op. Полный вывод этих трёх запусков сохранён в `profiles/benchmark-snapshot-review.txt`.

Правка `SaveBatch` также уменьшает временный snapshot в batch-сценарии, но исходный и повторный pprof-профили выше измеряют `Memory.Save`, который в этой правке не менялся. Поэтому файлы `base.pprof`, `result.pprof` и их diff остаются результатами именно профилирования `Memory.Save`, а не новой batch-оптимизации.

## HTTPS

По умолчанию сервис продолжает принимать обычные HTTP-запросы. HTTPS включается флагом `-s` или переменной окружения `ENABLE_HTTPS=true`:

```bash
go run ./cmd/shortener -a localhost:8443 -s
ENABLE_HTTPS=true go run ./cmd/shortener -a localhost:8443
```

При запуске сервер создаёт в памяти самоподписанный ECDSA-сертификат для `localhost`, `127.0.0.1` и host из адреса сервера. Сертификат предназначен для локальной разработки: браузер и `curl` по умолчанию не доверяют ему. Для ручной локальной проверки можно использовать `curl -k https://localhost:8443/ping`; в production следует использовать сертификат доверенного центра и внешний TLS termination.

`BASE_URL` настраивается независимо. Чтобы сервис возвращал короткие HTTPS-ссылки, его нужно явно задать с `https://`, например `BASE_URL=https://localhost:8443`.

## Конфигурация из JSON

Путь к JSON-файлу задаётся через `-c`, `-config` или переменную окружения `CONFIG`:

```bash
go run ./cmd/shortener -c config.json
go run ./cmd/shortener -config config.json
CONFIG=config.json go run ./cmd/shortener
```

Поддерживается полный набор runtime-настроек:

```json
{
  "server_address": "localhost:8443",
  "base_url": "https://localhost:8443",
  "file_storage_path": "/tmp/shortener.json",
  "database_dsn": "",
  "enable_https": true,
  "audit_file": "/tmp/shortener-audit.jsonl",
  "audit_url": ""
}
```

Приоритет источников: переменные окружения, явно переданные CLI-флаги, JSON-файл, значения по умолчанию. Поэтому `-s=false` может выключить HTTPS, включённый в JSON, а `ENABLE_HTTPS=true` снова перекроет флаг. `ENABLE_HTTPS` принимает стандартные boolean-значения Go: `true`, `false`, `1` и `0`. Неизвестные поля, некорректный JSON и указанный, но недоступный файл считаются ошибкой запуска. Если путь к файлу не задан, сервис работает с флагами, окружением и прежними значениями по умолчанию.

## Штатное завершение

Сервис обрабатывает `SIGTERM`, `SIGINT` и `SIGQUIT` как запрос на штатное завершение. HTTP- и HTTPS-сервер используют общий `http.Server`: после сигнала listener перестаёт принимать соединения, а активным запросам даётся до 30 секунд на завершение через `Shutdown` с отдельным контекстом. Если этот срок истёк, активные соединения принудительно закрываются, а `run()` возвращает ошибку вместо сообщения об успешной остановке.

После завершения HTTP-запросов `DeleteQueue` перестаёт принимать задачи, объединяет оставшиеся ID по пользователям и сохраняет последний неполный batch. File storage пишет изменения синхронно, поэтому после ожидания очереди отдельный фиктивный flush не требуется. Затем дренируется bounded-очередь аудита (с её десятисекундным пределом), после чего закрывается файловый observer. Соединение с PostgreSQL закрывается в `run()` последним, уже после возврата сервера и завершения worker-ов; logger остаётся доступен для сообщений об ошибках остановки.

## Внутренняя статистика

`GET /api/internal/stats` возвращает число сохранённых коротких ссылок и пользователей:

```json
{"urls":100,"users":10}
```

Доступ разрешён только адресу из заголовка `X-Real-IP`, если он входит в доверенную подсеть. Подсеть задаётся в формате CIDR через `-t`, `TRUSTED_SUBNET` или поле JSON `trusted_subnet`:

```bash
go run ./cmd/shortener -t 192.168.1.0/24
TRUSTED_SUBNET=192.168.1.0/24 go run ./cmd/shortener
```

```json
{"trusted_subnet":"192.168.1.0/24"}
```

Пустая подсеть запрещает доступ всем. Некорректный CIDR считается ошибкой конфигурации. Счётчик `urls` включает soft-deleted ссылки, поскольку отражает общее число созданных сокращений; `users` считает уникальные непустые идентификаторы владельцев ссылок.

## gRPC API

HTTP и gRPC используют одно хранилище и общую бизнес-логику. По умолчанию gRPC слушает `localhost:3200`; адрес можно изменить через `-g`, `GRPC_ADDRESS` или поле JSON `grpc_address`. Явное пустое значение отключает gRPC:

```bash
go run ./cmd/shortener -a localhost:8080 -g localhost:3200
GRPC_ADDRESS=localhost:3200 go run ./cmd/shortener
```

```json
{
  "server_address": "localhost:8080",
  "grpc_address": "localhost:3200"
}
```

Контракт находится в `internal/proto/shortener.proto` и предоставляет методы:

- `ShortenURL` — валидирует и сокращает URL; повторный URL идемпотентно возвращает уже существующую короткую ссылку;
- `ExpandURL` — возвращает оригинальный URL по short ID;
- `ListUserURLs` — возвращает активные ссылки текущего пользователя.

Все RPC требуют metadata `authorization` с сырым подписанным значением cookie `user_id`, полученным через HTTP API. Префикс `Bearer` не используется:

```text
authorization: dXNlci0x.<signature>
```

Некорректные аргументы возвращают `InvalidArgument`, отсутствующая или неверная подпись — `Unauthenticated`, неизвестная либо удалённая ссылка — `NotFound`, ошибки хранилища — `Internal`. Внутренние тексты SQL-ошибок клиенту не передаются.

При включении `-s` gRPC использует тот же созданный в памяти самоподписанный сертификат, что и HTTPS. Клиенту нужно добавить этот сертификат в доверенные корни; отключать проверку TLS не требуется. По сигналу HTTP и gRPC останавливаются одновременно: gRPC сначала выполняет `GracefulStop`, а после общего 30-секундного таймаута использует `Stop` и возвращает ошибку завершения.

Go-файлы protobuf сгенерированы `protoc 36.2`, `protoc-gen-go v1.36.12` и `protoc-gen-go-grpc v1.6.2`. После установки этих версий генерация из корня проекта воспроизводится командой:

```bash
protoc -I internal/proto -I /path/to/protoc/include \
  --go_out=internal/proto --go_opt=paths=source_relative \
  --go-grpc_out=internal/proto --go-grpc_opt=paths=source_relative \
  internal/proto/shortener.proto
```
