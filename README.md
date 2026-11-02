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
