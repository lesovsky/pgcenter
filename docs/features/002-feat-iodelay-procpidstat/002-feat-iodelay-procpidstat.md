---
created: 2026-05-19
status: draft
type: feature
size: S
---

# User Spec: iodelay в S-скрине (per-process IO wait)

## Что делаем

Добавляем две колонки в S-скрин (Shift+S, per-process stats): `iodelay_total,s` — накопленное
время ожидания блочного IO в формате `HH:MM:SS`, и `%iodelay` — доля времени, проведённого
в D-state между тиками, в процентах. Источник данных — `/proc/[pid]/stat` поле 42
(`delayacct_blkio_ticks`; ядро нумерует поля с 1, в коде это `suffix[39]` после отбрасывания
`pid` и `comm`). Итого экран показывает 19 колонок вместо 17.

## Зачем

DBA видит backend с высоким IO throughput (`read,KiB/s`, `write,KiB/s`), но не понимает,
реально ли процесс ждёт диска или работает через page cache. `%iodelay` показывает именно это:
какой процент времени конкретный backend провёл заблокированным в D-state в ожидании блочного IO.
Это позволяет за секунды отличить IO-bound backend от CPU-bound или cache-bound — без выхода
из pgcenter и без запуска `iotop`.

## Пользовательские истории

- Как DBA, я хочу видеть `%iodelay` для каждого PostgreSQL backend, чтобы определить, какой
  процесс реально ждёт диска, а не просто делает IO через page cache.
- Как DBA, я хочу понимать, почему колонки iodelay пустые, чтобы не думать что это баг
  pgcenter, а знать что нужно включить delay accounting в ядре.

### Пользовательские сценарии

**Сценарий 1: Диагностика IO-bound backend**
1. DBA замечает медленный запрос в PostgreSQL — клиенты жалуются на задержки.
2. Запускает pgcenter, нажимает `Shift+S` — открывается S-скрин.
3. Видит backend с `%iodelay=72%` и `iodelay_total=00:04:12`, при этом `%all=3%`.
4. Вывод очевиден: процесс почти не нагружает CPU, но большую часть времени ждёт диска.
5. Переключается на анализ дисковой подсистемы — IOPS, queue depth, storage latency.

**Сценарий 2: Delay accounting недоступен**
1. DBA нажимает `Shift+S`, в системе `kernel.task_delayacct=0`.
2. В cmdline area pgcenter показывает предупреждение:
   `"iodelay and IO stats unavailable: task_delayacct=0; run: sysctl -w kernel.task_delayacct=1, then re-open screen"`
   (если одновременно недоступен и IO — комбинированное сообщение; если только delayacct —
   только про delayacct).
3. Колонки `iodelay_total,s` и `%iodelay` отображают `""` для всех строк.
4. DBA понимает причину и знает как исправить: выполнить `sysctl -w kernel.task_delayacct=1`
   и нажать `Shift+S` ещё раз для re-probe.

## Дизайн и интерфейс

### S-скрин (Shift+S)

Экран отображает 19 колонок в следующем порядке:

| # | Имя | Описание |
|---|-----|----------|
| 1 | `pid` | PID backend |
| 2 | `datname` | База данных |
| 3 | `usename` | Пользователь |
| 4 | `state` | Состояние |
| 5 | `wait_etype` | Тип wait event |
| 6 | `wait_event` | Wait event |
| 7 | `all_total,s` | Накопленное CPU время (user+sys), HH:MM:SS |
| 8 | `us_total,s` | Накопленное user CPU, HH:MM:SS |
| 9 | `sy_total,s` | Накопленное sys CPU, HH:MM:SS |
| 10 | `read_total,KiB` | Всего прочитано с диска, KiB |
| 11 | `write_total,KiB` | Всего записано на диск, KiB |
| 12 | **`iodelay_total,s`** | Накопленное время IO-ожидания, HH:MM:SS *(новая)* |
| 13 | `%all` | CPU utilization (user+sys), % |
| 14 | `%us` | User CPU, % |
| 15 | `%sy` | Sys CPU, % |
| 16 | `read,KiB/s` | IO read rate, KiB/s |
| 17 | `write,KiB/s` | IO write rate, KiB/s |
| 18 | **`%iodelay`** | IO wait rate, % *(новая)* |
| 19 | `query` | Текст запроса |

### UX-поведение

- **Доступен delay accounting** (`task_delayacct=1`): колонки отображают реальные значения.
  На первом тике `iodelay_total,s` показывает накопленное значение, `%iodelay = ""` (нет prev).
- **Недоступен delay accounting** (`task_delayacct=0` или файл отсутствует):
  обе колонки `""` + предупреждение в cmdline area при открытии экрана.
- **Оба недоступны** (IO и delayacct): одно комбинированное предупреждение.
- **`%iodelay` может превышать 100%**: это ожидаемо — метрика не нормализуется на число CPU
  (wall-clock blocked time, не CPU utilization). Например, 4-ядерная машина при полном IO-блоке
  покажет 100%, одна нить при IO-ожидании — до 100%.
- Probe делается один раз при нажатии `Shift+S`. Если sysctl изменился после открытия экрана —
  нужно закрыть и переоткрыть (`Shift+S` → другой экран → `Shift+S`).

## Как должно работать

### Основной сценарий

1. Пользователь нажимает `Shift+S`.
2. pgcenter читает `/proc/sys/kernel/task_delayacct` — определяет доступность.
3. Если доступно: на каждом тике читает `suffix[39]` из `/proc/[pid]/stat` для каждого PID.
4. Вычисляет: `iodelay_total,s = formatCPUTime(IODelay, ticks)`,
   `%iodelay = ΔIOD / (itv × ticks) × 100` (не нормализуется на cpuCount).
5. Отображает 19 колонок с новыми значениями.

### Граничные случаи

- **Первый тик** (нет предыдущего сэмпла): `iodelay_total,s` показывает текущее накопленное
  значение; `%iodelay = ""` — нет предыдущего сэмпла для вычисления дельты.
- **PID исчез между тиками** (backend завершился): `iodelay_total,s = "00:00:00"`,
  `%iodelay = "0.00"` — `IODelay` в текущем сэмпле равен 0, предыдущий сэмпл есть,
  дельта нулевая. Поведение аналогично CPU-колонкам (те же условия, тот же файл-источник).
- **`/proc/[pid]/stat` содержит меньше 40 полей** (очень старое ядро): `IODelay = 0`, без паники.
- **`/proc/sys/kernel/task_delayacct` отсутствует** (ядро без `CONFIG_TASK_DELAY_ACCT`):
  probe возвращает false, колонки `""`, предупреждение.
- **`task_delayacct` переключили в runtime** после открытия экрана: значения останутся некорректными
  до переоткрытия экрана (re-probe не выполняется автоматически).

## Критерии приёмки

- [ ] S-скрин отображает 19 колонок (было 17)
- [ ] `iodelay_total,s` показывает накопленное IO-ожидание в формате `HH:MM:SS`
- [ ] `%iodelay` показывает процент с 2 знаками после запятой
- [ ] При `kernel.task_delayacct=0`: обе iodelay-колонки `""` + предупреждение в cmdline area
- [ ] При `kernel.task_delayacct=1`: iodelay-колонки показывают реальные значения
- [ ] Первый тик после `Shift+S`: `iodelay_total,s` = накопленное значение, `%iodelay = ""`
- [ ] `%iodelay` не нормализуется на число CPU: при полной IO-блокировке одной нити `%iodelay ≈ 100%` независимо от числа ядер (unit-тест с `cpuCount=4` и `delayDelta = itv*ticks` должен давать `100.00`, не `25.00`)
- [ ] `make test && make lint && make vuln` проходят без ошибок
- [ ] Tech debt `[001]` помечен как resolved в `docs/tech-debt.md`

## Ограничения

- Только Linux, только local mode — procfs недоступен для удалённых PostgreSQL-соединений.
- Требует `CONFIG_TASK_DELAY_ACCT=y` в ядре и `kernel.task_delayacct=1` (runtime sysctl).
- Probe выполняется один раз при открытии S-скрина — runtime изменение sysctl не подхватывается.
- `%iodelay` может превышать 100% — это корректное поведение, не баг.
- S-скрин не поддерживает `pgcenter record` / `pgcenter report` (существующее ограничение).

## Риски

- **Ncols 17→19:** расхождение между view config и фактическим числом колонок вызывает
  panic в `align.SetAlign()`. **Митигация:** обновить `Ncols` в `view.go` и `procPidResultNcols`
  синхронно; покрыто тестом на количество колонок.
- **`printCmdline` mutual exclusion:** IO-warning и delayacct-warning нельзя показать
  одновременно. **Митигация:** комбинированное сообщение когда оба недоступны (вариант C,
  подтверждён).

## Технические решения

- Источник данных — `/proc/[pid]/stat` поле 42 (`delayacct_blkio_ticks`), не Netlink taskstats.
  Netlink отвергнут: требует новой dependency и значительно расширяет scope.
- Availability probe — `/proc/sys/kernel/task_delayacct` sysctl (читается без root, не требует PID).
  Это авторитетный runtime-источник состояния delay accounting.
- `%iodelay` не нормализуется на `cpuCount` — это wall-clock blocked time, а не CPU utilization.
  CPU-метрики (`%all`, `%us`, `%sy`) нормализуются потому что показывают использование процессора.
- `%iodelay` не входит в `%all` — CPU running и IO blocking взаимоисключающие состояния.
- `formatCPUTime(jiffies, ticks)` переиспользуется для `iodelay_total,s` — функция уже есть.
- `IODelay` добавляется в `ProcPidStat` — новые Collector maps не нужны, значения едут
  в существующих `prevProcPidStats` / `currProcPidStats`.

## Тестирование

**Unit-тесты:** делаются всегда.

- Обновить `TestReadProcPidStatFile`: golden file с ненулевым `suffix[39]`.
- Обновить `TestBuildProcPidResult`: ожидать 19 колонок, проверить значения iodelay-колонок.
- Добавить `TestCheckDelayAcctAvailable`.
- Добавить `TestBuildProcPidResult_DelayAvailable` и `_DelayUnavailable`.

**Интеграционные тесты:** не делаем — procfs данные недетерминированы в CI.

**E2E тесты:** не делаем — фича S-размера, ручная проверка достаточна.

## Как проверить

### Агент проверяет

| Шаг | Инструмент | Ожидаемый результат |
|-----|-----------|-------------------|
| 1. `make build` | bash | Сборка без ошибок |
| 2. `make test` | bash | Все тесты зелёные, включая новые iodelay-тесты |
| 3. `make lint` | bash | Нет lint-ошибок |
| 4. `make vuln` | bash | Нет known vulnerabilities |

### Пользователь проверяет

- **Позитивный сценарий:** `sysctl -w kernel.task_delayacct=1` → запустить pgcenter →
  `Shift+S` → выполнить IO-нагружающий запрос (`SELECT * FROM большая_таблица`) →
  убедиться что `%iodelay > 0` и `iodelay_total,s` меняется между тиками.
- **Негативный сценарий:** `sysctl -w kernel.task_delayacct=0` → `Shift+S` →
  убедиться что iodelay-колонки `""` и в cmdline area появляется предупреждение.
- **Первый тик:** сразу после открытия `Shift+S` — `%iodelay` должна быть `""`,
  `iodelay_total,s` — ненулевое `HH:MM:SS`.

## Post-implementation

<!-- This section is filled automatically by /done during feature finalization.
     It captures divergences between the original spec and the actual result.
     DO NOT fill manually — this is maintained by the reconciliation process. -->
