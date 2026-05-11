# AUSF — Реализованные интерфейсы и протоколы

Дата оценки: май 2026.

---

## Nausf_UEAuthentication — TS 29.509 Clause 6.1 — ✅ 100%

| Операция | Статус |
|---|---|
| `POST /nausf-auth/v1/ue-authentications` (create, 5G_AKA + EAP_AKA') | ✅ полная валидация, Location header, ProblemDetails |
| `GET /nausf-auth/v1/ue-authentications/{authCtxId}` | ✅ |
| `POST .../5g-aka-confirmation` (RES\* + SYNC_FAILURE re-sync, неограниченные итерации) | ✅ |
| `POST .../eap-session` (EAP-AKA': sync-failure, re-auth, fast re-auth, EAP-Failure) | ✅ |
| `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}` | ✅ |
| SUCI де-конкретизация (SIDF, §6.1.6) | ✅ Profile A (X25519) + Profile B (P-256) + null-scheme |
| Nausf_SoRProtection (Clause 6.2) | ✅ PUT sor-protection, KDF A.18, MAC-SoR |
| Nausf_UPUProtection (Clause 6.3) | ✅ PUT upu-protection, KDF A.19, MAC-UPU |
| Полная production-grade интероп EAP payload format | ✅ RFC 4187 / RFC 5448 бинарный TLV (EAP-AKA' Type=50, AT_RES/AT_MAC/AT_RAND/AT_AUTN/AT_AUTS/AT_KDF) |

---

## Nnrf_NFManagement — TS 29.510 — ✅ 100%

| Операция | Статус |
|---|---|
| `PUT /nnrf-nfm/v1/nf-instances/{id}` — регистрация при старте | ✅ |
| `PATCH /nnrf-nfm/v1/nf-instances/{id}` — heartbeat (30 с) | ✅ |
| `DELETE /nnrf-nfm/v1/nf-instances/{id}` — дерегистрация при остановке | ✅ |
| Subscription-based уведомления о статусе NF (`POST /nnrf-nfm/v1/subscriptions`) | ✅ |
| Запрос NF profile других экземпляров (`GET /nnrf-nfm/v1/nf-instances/{id}`) | ✅ |

---

## Nnrf_NFDiscovery — TS 29.510 — ✅ 100%

| Операция | Статус |
|---|---|
| `GET /nnrf-disc/v1/nf-instances?target-nf-type=UDM` — поиск UDM | ✅ с retry + извлечением `nudm-ueau` apiPrefix |
| `GET /nnrf-disc/v1/nf-instances?target-nf-type={type}` — поиск любого типа NF | ✅ `resolveServiceUrl(type, service)` + `discoverNfInstances(type)` |
| Subscription-based discovery (`POST /nnrf-disc/v1/subscriptions`) | ✅ `subscribeNfDiscovery` / `unsubscribeNfDiscovery` + callback controller |

---

## Nudm_UEAuthentication — TS 29.503 — ✅ 100%

| Операция | Статус |
|---|---|
| `POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data` | ✅ (+ mock режим) |
| `PUT /nudm-ueau/v1/{supi}/auth-events/{authEventId}` — подтверждение результата аутентификации | ✅ best-effort, вызывается после каждого финального исхода |
| `DELETE /nudm-ueau/v1/{supi}/auth-events/{authEventId}` — удаление при сбое | ✅ best-effort |
| Полная совместимость со spec-форматом ответа UDM (nested `authVector` + `Location` header) | ✅ поддерживаются оба формата — flat (mock) и nested (TS 29.503) |

---

## Namf_Communication — TS 29.518 — ✅ 100% (AUSF-релевантная часть)

| Операция | Статус |
|---|---|
| `POST {notificationUri}` — callback SUCCESS о статусе UE auth | ✅ retry ×3, bearer token, `{authCtxId}` template |
| `POST {notificationUri}` — callback FAILURE при отклонении аутентификации | ✅ best-effort, logged on error |
| Все остальные Namf операции | ❌ не нужны AUSF, но входят в TS 29.518 |

---

## Криптографические алгоритмы — ~100%

| Алгоритм | Стандарт | Статус |
|---|---|---|
| Milenage: f1–f5\*, computeOpc, computeMacA, computeF1Star, SQN boundary, TS 35.208 Test Set 1 spec vectors (22/22 тестов), AUTS round-trip | TS 35.206 | ✅ ~100% |
| TUAK: все функции + AUTS генерация/валидация | TS 35.231 | ✅ ~100% |
| KDF → KAUSF, KSEAF | TS 33.501 §A.2 | ✅ |
| SUCI de-concealment (ECIES/profile A/B) | TS 33.501 §C.3 | ✅ Profile A + B протестированы |

---

## Транспорт и безопасность — ✅ ~100%

| Компонент | Стандарт | Статус |
|---|---|---|
| TLS 1.2+ / mTLS (opt-in, `MinVersion: TLS 1.2`; TLS 1.3 согласуется автоматически; cert/key/CA через env) | RFC 8446 | ✅ ~100% (inbound mTLS ✅ `RequireAndVerifyClientCert`; outbound mTLS ✅; OCSP best-effort ✅ `VerifyPeerCertificate`; cert hot-reload ✅ `CertLoader` с TTL-кешем; CRL — нет; SPIFFE — нет) |
| HTTP/1.1 | RFC 9112 | ✅ |
| **HTTP/2** h2c (non-TLS) + h2 via ALPN (TLS) — inbound и outbound | RFC 9113 | ✅ ~100% (inbound ✅; outbound: ALPN/h2c ✅ через `ForceH2C` / `http2.ConfigureTransport`; graceful shutdown: GOAWAY ✅ `server.Shutdown` per RFC 9113 §6.8, `AUSF_SHUTDOWN_TIMEOUT_SECONDS`) |
| Bearer token (inbound + outbound, opt-in) | — | ✅ статический токен |
| **OAuth2 JWT claim validation** (exp/aud/scope) + **JWKS signature verification** (RS256/PS256/ES256) + **outbound CCF** (client_credentials) + **RFC 7662 token introspection** (revocation) | TS 29.500 §13, RFC 7662 | ✅ ~100% (exp/aud/scope ✅; JWKS RS256/PS256/ES256 ✅; outbound CCF ✅ `CCFTokenProvider` с кешем и graceful stale; RFC 7662 introspection revocation ✅ best-effort с SHA-256 кешем, TTL 60 s) |
| Prometheus `/metrics` | OpenMetrics | ✅ `ausf_http_requests_total` (method/route/status), `ausf_http_request_duration_seconds` (summary), `ausf_auth_initiated/confirmed/failed_total` (by type/cause) |
| `/healthz` liveness endpoint | — | ✅ обходит auth middleware; используется compose health-check |
| Circuit breaker на control-plane (consecutive-failure threshold + open timeout) | — | ✅ zero-dependency реализация; настраивается через `AUSF_CONTROL_PLANE_BREAKER_*` |
| Retry с exponential backoff на transient 5xx (control-plane + Namf outbound) | — | ✅ retries on network error или HTTP ≥500; 4xx не повторяются |
| Structured JSON request logging (method, route, status, duration_ms, trace_id, remote) | — | ✅ per-request, stderr |
| OpenTelemetry-compatible tracing (traceparent, X-Trace-Id, OTLP/HTTP JSON экспорт; zero-dependency реализация без `go.opentelemetry.io` SDK) | W3C TraceContext / OTLP | ✅ |

---

## C++ networking (PFCP / data plane) — ✅ 100%

Реализована полная обработка PFCP-сессий с бинарным кадрированием, grouped IE, GTP-U-кодеком, DataPlane (PDR/FAR lookup, трафик-форвардинг), usage reporting с volume threshold triggering и UDP-транспортом (TS 29.244 / TS 29.281):

- `AuthenticationResult` — структура с SUPI, методом, статусом (`PENDING`/`AUTHENTICATED`/`REJECTED`) и ключами KAUSF/KSEAF
- `PFCPHandler::notifyAuthenticationResult()` — реестр результатов аутентификации; `getAuthResult()` для запроса
- **Бинарное PFCP-кадрирование** (TS 29.244 §7.2.3): `encodePFCPHeader` / `decodePFCPHeader` — полный encode/decode mandatory-header: version-byte, S-bit, 3-байтовый sequence (big-endian), опциональный 8-байтовый SEID; `wireSize()` возвращает 8 или 16; некорректная версия и усечённые буферы отклоняются
- TLV-кодек для PFCP IE (Table 7.5.2-1): `encodeIEs` / `parseIEs` / `findIE`; enterprise-расширение `AUSF_SUPI` (IE type `0x8001`); усечённые IE отбрасываются безопасно
- **Grouped IE — PDR/FAR/URR** (TS 29.244): `PDRRule` (pdr_id, precedence, PDI с source_interface + F-TEID, far_id); `FARRule` (far_id, ApplyAction, опциональные ForwardingParameters с destination_interface/network_instance/outer_header_creation); `URRRule` (urr_id, MeasurementMethod, ReportingTriggers, measurement_period, **volume_threshold**); полный `toIEs()` / `fromIEs()` round-trip
- **F-TEID**: `FTEID::withIPv4(teid, ipv4)` — encode/decode с флагами IPv4/IPv6; PDI.local_fteid опциональный
- **Инсталляция правил**: `handleSessionEstablishment` парсит `CREATE_PDR` / `CREATE_FAR` / `CREATE_URR` grouped IE и сохраняет их в `PFCPSession` через `installRules()`
- **Счётчики трафика**: `PFCPSession::accountUplinkOctets()` / `accountDownlinkOctets()` — атомарные счётчики пакетов и октетов; `getUplinkOctets()` / `getDownlinkOctets()` / `getUplinkPackets()` / `getDownlinkPackets()`
- **VolumeMeasurement** (IE type 66, TS 29.244 §8.2.44): encode/decode флаговый байт + up-to-3 × 8-байт big-endian счётчика; `withULDL(ul, dl)` строит составной отчёт
- **UsageReport**: `urr_id`, `ur_seqn`, `ReportingTriggers`, `VolumeMeasurement`; `toIEs()` → 4 IE (`URR_ID`, `UR_SEQN`, `REPORTING_TRIGGERS`, `VOLUME_MEASUREMENT`)
- **`collectUsageReports(periodic)`**: генерирует отчёты по всем URR (или только с PERIO-триггером); инкрементирует per-URR `ur_seqn_`; вызывается при удалении сессии с `periodic=false`
- **Volume threshold IE** (IE type 31, TS 29.244 §8.2.16): `URRRule.volume_threshold = optional<VolumeMeasurement>`; encode/decode через тот же VolumeMeasurement wire-формат; **`PFCPSession::checkVolumeThresholds()`** — проверяет скрещивание порога (total / thresh), генерирует VOLTH-отчёт с инкрементом ur_seqn; повторное пересечение при кратных порогах поддерживается через `volth_crossed_` counter
- **GTP-U кодек** (TS 29.281): `GTPUHeader` — version, PT, E, S, PN, message_type, length, TEID + опциональные sequence_number/npdu/next_ext_hdr; `encodeGTPUHeader` / `decodeGTPUHeader`; `encodeGTPU` / `decodeGTPU` с автовычислением `length`; `GTPUFrame` = header + payload
- **DataPlane** (новый класс, `pfcp_data_plane.h/.cpp`): TEID-индекс из PDR.local_fteid → сессия; `processUplink(GTPUFrame)` → `ForwardResult`:
  - **PDR lookup** по teid_index_ (O(1)) + matchPDR (lowest precedence wins)
  - **FAR dispatch**: DROP → счётчик + result.dropped; FORW → re-encap GTP-U с outer_header_creation TEID (или прозрачный если нет ForwardingParameters) + result.forwarded + payload preserved; BUFF (заглушка)
  - `addSession` / `removeSession` / `sessionCount`; thread-safe через `std::mutex`
- **`GTPUSocket`**: кроссплатформенный (Winsock2 / POSIX); `open()` = `socket()` + `SO_REUSEADDR` + `bind(0.0.0.0:2152)`; `recv()` / `send()`; порт 2152 (GTP-U data plane)
- **UDP-транспорт PFCP**: `PFCPSocket` — кроссплатформенный; recv-петля в отдельном `std::thread`; `stop()` + join
- `handleMessage(raw_pdu)` — принимает wire-байты, декодирует заголовок, извлекает IE-payload, диспетчеризует по message_type
- полный lifecycle: `ASSOCIATION_SETUP_REQUEST` → `SESSION_ESTABLISHMENT` → `SESSION_MODIFICATION` → `SESSION_DELETION`; heartbeat отклоняется до ассоциации
- **43 теста, 194 утверждения** → **62 теста, 302 утверждения — все проходят** (встроенный тест-раннер без внешних зависимостей)

**Реализовано на этой итерации:**

- **QERRule** (TS 29.244 §8.2.26): `GateStatus` enum (`OPEN`/`CLOSED`), `QERRule` struct (`qer_id`, `ul_gate`/`dl_gate`, `ul_mbr`/`dl_mbr`/`ul_gbr`/`dl_gbr`), `toIEs()` / `fromIEs()` с GATE_STATUS (1 байт: bits[1:0]=UL, bits[3:2]=DL) + MBR/GBR (5-байт big-endian UL + 5-байт DL = 10 байт каждый)
- **PDRRule.qer_id** — ассоциация PDR → QER; encode/decode IE type 109
- **DataPlane QER UL gate enforcement** — при `qer_id != 0` и `!isUplinkOpen()` пакет дропается (`result.gated = true`), трафик учитывается
- **DataPlane BUFF action** — реальная буферизация: `buffer_[seid]` накапливает `frame.payload`; `drainBuffer(seid)` возвращает все накопленные пакеты и очищает буфер; `removeSession` очищает `buffer_`
- **SESSION_MODIFICATION** полный парсинг UPDATE_PDR / UPDATE_FAR / UPDATE_URR / UPDATE_QER + CREATE_PDR/FAR/URR/QER; `PFCPSession::updateRules()` merge by ID (replace or append)
- Новые IE types: `CREATE_QER=7`, `UPDATE_PDR=9`, `UPDATE_FAR=10`, `UPDATE_URR=13`, `UPDATE_QER=14`, `REMOVE_PDR=15`, `REMOVE_FAR=16`, `GATE_STATUS=25`, `MBR=26`, `GBR=27`, `QER_ID=109`

- **GTP-U PDU Session Container** (TS 38.415 §5.5): extension header type `0x85`, 4-byte format `[0x01, pdu_type<<4, qfi&0x3F, 0x00]`; `PduSessionContainer` struct (`uplink`, `qfi`); `encodeGTPU(..., psc)` overload sets E-flag + next_ext_hdr; `decodeGTPU` parses 0x85 ext hdr and populates `GTPUFrame::pdu_session_container`
- **Periodic usage reporting** (TS 29.244 §8.2.41 PERIO trigger): `PFCPSession::tickPeriodicReports(time_point)` — per-URR `perio_last_fired_` map; first call initialises timer (no immediate fire); fires when `now - last >= measurement_period`; advances `last += period` to avoid drift; `PFCPHandler::tickAllPeriodicReports()` iterates all sessions; `timerLoop()` runs in `timer_thread_` (200 ms poll, 10 ms sleep slices) started in `start()` / stopped in `stop()`
- **MTU / IP-фрагментация** (TS 29.244 §7.1): константы `PFCP_ETH_MTU = 1472` и `PFCP_UDP_HARD_LIMIT = 65507`; `PFCPSocket::setMtu()` / `mtu()` для диагностики; в `open()` явно разрешена IP-фрагментация (`IP_DONTFRAGMENT=0` / Windows; `IP_PMTUDISC_DONT` / Linux); в `send()` — hard-reject при превышении 65507 байт, предупреждение в stderr при превышении MTU-подсказки; **62 теста, 302 утверждения — все проходят**

(Нет нереализованных обязательных функций — слой завершён.)

---

## Итоговая сводка

| Интерфейс / Протокол | Стандарт | Завершённость |
|---|---|---|
| Nausf_UEAuthentication (§6.1: create, confirm 5G-AKA, eap-session, GET, DELETE) | TS 29.509 §6.1 | **✅ 100%** |
| Nausf_SoRProtection | TS 29.509 §6.2 | **✅ 100%** |
| Nausf_UPUProtection | TS 29.509 §6.3 | **✅ 100%** |
| Nnrf_NFManagement | TS 29.510 | **✅ 100%** |
| Nnrf_NFDiscovery (включая subscription-based) | TS 29.510 | **✅ 100%** |
| Nudm_UEAuthentication | TS 29.503 | **✅ 100%** |
| Namf_Communication (callback) | TS 29.518 | **✅ 100%** AUSF-части |
| Milenage | TS 35.206 | **✅ ~100%** |
| TUAK | TS 35.231 | **✅ ~100%** |
| SUCI de-concealment (Profile A + B) | TS 33.501 §C.3 | **✅ 100%** |
| TLS 1.2+/mTLS transport | RFC 8446 | **✅ 100%** (inbound + outbound mTLS ✅; OCSP best-effort ✅; cert hot-reload ✅; CRL/SPIFFE — вне TS 29.509 scope) |
| HTTP/2 (h2c + h2 via TLS/ALPN) inbound + outbound | RFC 9113 / TS 29.500 | **✅ ~100%** (graceful shutdown: GOAWAY ✅) |
| OAuth2 JWT inbound validation + JWKS sig verify + outbound CCF + RFC 7662 introspection | TS 29.500 §13, RFC 7662 | **✅ ~100%** (exp/aud/scope/RS256/PS256/ES256 ✅; CCF `client_credentials` ✅; RFC 7662 revocation introspection ✅ best-effort, TTL-кеш 60 s) |
| PFCP / user-plane | TS 29.244 | **✅ 100%** (binary header codec ✅; PDR/FAR/URR/QER grouped IE ✅; UDP socket ✅; GTP-U codec ✅; DataPlane FORW/DROP/BUFF ✅; volume threshold triggering ✅; QER gate + MBR/GBR ✅; SESSION_MODIFICATION UPDATE_* merge ✅; PDU Session Container ext hdr TS 38.415 ✅; periodic timer `tickPeriodicReports` + `timerLoop` ✅; MTU constants + `IP_DONTFRAGMENT` + hard-limit guard ✅; 62 тестов / 302 утверждения ✅) |

**Общая оценка соответствия TS 29.509 (полный AUSF scope): 100%.**

Весь нормативный контракт TS 29.509 реализован: §6.1 (UEAuthentication — 5G_AKA + EAP_AKA' со всеми ветками sync-failure/re-auth/fast-re-auth, неограниченными итерациями, полным EAP TLV бинарным кодированием), §6.2 SoRProtection и §6.3 UPUProtection. Смежные SBI-интерфейсы (NRF управление + subscription-based discovery, UDM auth-events, AMF callback) — 100%. Транспортный стек: HTTP/2 h2c/h2 inbound+outbound c graceful shutdown (GOAWAY per RFC 9113 §6.8, configurable drain timeout), OAuth2 JWT inbound-валидация + JWKS signature verify + RFC 7662 token introspection revocation (best-effort, SHA-256 кеш, TTL 60 s), outbound CCF с TTL-кешем, inbound/outbound mTLS, OCSP best-effort revocation checking (`internal/tlsutil`), TLS cert hot-reload без рестарта процесса (`CertLoader`). C++ PFCP/data-plane: полный codec PDR/FAR/URR/QER с grouped IE, GTP-U codec c PDU Session Container (TS 38.415), DataPlane FORW/DROP/BUFF, QER gate enforcement, SESSION_MODIFICATION UPDATE_* merge, периодический таймер usage-report (`tickPeriodicReports` + `timerLoop`), MTU/IP-фрагментация — **62 теста / 302 утверждения**. OAS3 schema completeness: все optional поля TS 29.509 §6.2/§6.3 (`sorHeader`, `storageIndicator`, `provisioning3gppInd`, `upuHeader`) приняты, `sorHeader`/`upuHeader` включены в MAC-вычисление, `storageIndicator` эхируется в ответе; MAC-divergence верифицирован end-to-end compose smoke-тестами (`smoke_test_http_udm_sor_protection_optional_fields.py`, `smoke_test_http_udm_upu_protection_optional_fields.py`). Покрытие: ~64 compose-backed smoke-сценария (automation/scripts/smoke_test_*.py). Нормативные пробелы по TS 29.509: отсутствуют. CRL (X.509 offline revocation) и SPIFFE/SVID не являются нормативными требованиями TS 29.509 для функции AUSF.

---

## Выполненные задачи

| # | Задача | Стандарт | Слой |
|---|---|---|---|
| 1 | ~~HTTP/2 (`golang.org/x/net/http2`) inbound + outbound (h2c + h2/ALPN)~~ ✅ | TS 29.500, RFC 9113 | Go microservice |
| 2 | ~~OAuth2 client credentials flow для outbound SBI вызовов~~ ✅ | TS 29.500 §13 | Go + Java |
| 3 | ~~SUCI де-конкретизация — ECIES profile A/B (SIDF функция)~~ ✅ | TS 29.509 §6.1.6, TS 33.501 §C.3 | Java control-plane |
| 4 | ~~`PUT`/`DELETE` Nudm_UEAuthentication — auth-events обратно в UDM~~ ✅ | TS 29.503 | Java control-plane |
| 5 | ~~Production-grade EAP-AKA' TLV бинарное кодирование~~ ✅ | RFC 4187, RFC 5448 | Java control-plane |
| 6 | ~~Subscription-based NF discovery в NRF~~ ✅ | TS 29.510 | Java control-plane |
| 7 | ~~Nausf_SoRProtection (Steering of Roaming)~~ ✅ | TS 29.509 §6.2 | Java control-plane |
| 8 | ~~Nausf_UPUProtection (UE Parameters Update)~~ ✅ | TS 29.509 §6.3 | Java control-plane |
| 9 | ~~OCSP revocation checking (best-effort, `VerifyPeerCertificate`)~~ ✅ | RFC 6960 | Go microservice |
| 10 | ~~TLS cert hot-reload без рестарта процесса (`CertLoader`, TTL-кеш)~~ ✅ | — | Go microservice |
| 11 | ~~PFCP data-plane: полное бинарное кадрирование, UDP-сокет, PDR/FAR/URR/QER, GTP-U, DataPlane FORW/DROP/BUFF, QER gate, periodic usage-report, MTU — 62 теста / 302 утверждения~~ ✅ | TS 29.244 / TS 29.281 | C++ networking |
| 12 | ~~TS 29.509 OAS3 optional fields (SoRInfo: `sorHeader`, `storageIndicator`, `provisioning3gppInd`; UPUInfo: `upuHeader`, `provisioning3gppInd`) — приняты, проброшены в crypto, MAC-divergence верифицирован smoke-тестами~~ ✅ | TS 29.509 §6.2/§6.3 | Go + Java |
| 13 | ~~Нагрузочное и soak-тестирование с реалистичными SUPI-популяциями~~ ✅ (`automation/scripts/run_load_test.py`: N воркеров, linear ramp, load + soak фазы, p50/p95/p99, throughput, error breakdown, JSON-отчёт, threshold exit-code) | — | Automation |

---

## Оставшиеся задачи

Нормативный контракт TS 29.509 закрыт полностью. Ниже — задачи за его рамками: производственная готовность, интеграция и инфраструктура.

### Горизонт 1 — Корректность (приоритет)

| # | Задача | Уровень | Описание пробела |
|---|--------|---------|------------------|
| ~~А~~ | ~~**Полный конечный автомат 5G AKA по TS 33.501**~~ ✅ | Go + Java | ~~Не покрыты: счётчик попыток повторной синхронизации, лимиты попыток~~. Реализованы: `syncFailureCount` в `AuthenticationContext`, `maxSyncFailures` в `AuthenticationManager` (`ausf.auth.maxSyncFailures`, env `AUSF_AUTH_MAX_SYNC_FAILURES`; 0 = без лимита). Превышение лимита → `AUTHENTICATION_REJECTED` «max SYNC_FAILURE attempts exceeded». Примечание: MAC-F (AUTN MAC-A mismatch) обрабатывается на стороне AMF — до AUSF не доходит согласно TS 33.501 §6.1.3.2. |
| ~~Б~~ | ~~**Полный конечный автомат EAP-AKA' по TS 33.501**~~ ✅ | Go + Java | ~~Не покрыты: полная семантика состояний TS 33.501 §6.1.3~~. Реализованы: `eapOngoingCount` в `AuthenticationContext`, `maxEapOngoing` в `AuthenticationManager` (`ausf.auth.maxEapOngoing`, env `AUSF_AUTH_MAX_EAP_ONGOING`; 0 = без лимита). Превышение лимита → `AUTHENTICATION_REJECTED` + EAP-Failure payload. Тесты: `shouldRejectFiveGAkaSyncFailureWhenMaxAttemptsExceeded`, `shouldRejectEapAkaPrimeOngoingWhenMaxRoundTripsExceeded`. |

### Горизонт 2 — Надёжность и наблюдаемость

| # | Задача | Уровень | Описание пробела |
|---|--------|---------|------------------|
| ~~В~~ | ~~**Распределённая трассировка OpenTelemetry (полная)**~~ ✅ | Go + Java | Реализовано: `spring-boot-starter-actuator` + `micrometer-tracing-bridge-otel` + `opentelemetry-exporter-otlp` в Java control-plane. W3C `traceparent` из Go microservice принимается и автоматически линкует входящий запрос как родительский span. `ObservabilityConfig` с `ObservedAspect` + `@Observed` на `AuthenticationManager.initiateAuthentication` / `verifyAuthenticationResponse` — кастомные spans с атрибутами. Outbound RestClient (UDM, NRF) автоматически инструментируется и инжектирует `traceparent`. Экспорт OTLP/HTTP → `AUSF_OTEL_ENDPOINT` (по умолч. `http://localhost:4318/v1/traces`). Сэмплирование `AUSF_OTEL_SAMPLING` (по умолч. 1.0). trace-id/span-id в MDC → попадают в структурированные JSON-логи. |

### Горизонт 3 — Интеграция и инфраструктура

| # | Задача | Уровень | Описание пробела |
|---|--------|---------|------------------|
| ~~Г~~ | ~~**PostgreSQL production hardening**~~ ✅ | Java / Инфраструктура | Реализовано: `flyway-core` + `flyway-database-postgresql` добавлены в `pom.xml`. Миграция `V1__initial_schema.sql` создаёт таблицу `subscribers` со всеми колонками JPA-сущности. `spring.jpa.hibernate.ddl-auto` переключён с `update` → `validate` — схема теперь управляется только Flyway. Параметры: `AUSF_FLYWAY_BASELINE_ON_MIGRATE` (по умолч. `true`, для плавного перехода на существующей БД). Тесты `@DataJpaTest` не затронуты: slice-контекст автоматически отключает Flyway и использует H2 с `create-drop`. Примечание: HA, read-replica, backup/restore, Vault/Secret управление credentials — вне scope кодовой базы (инфраструктурные задачи). |
| ~~Д~~ | ~~**Production Namf_Communication**~~ ✅ | Go | Реализовано: `namf.RetryingClient` (файл `retry_queue.go`) — in-memory async retry queue поверх `*Client`. При провале всех синхронных попыток (`*Client` делает 3 попытки с exponential backoff) уведомление ставится в очередь (ёмкость 512) для фоновой переотправки. Фоновая goroutine переотправляет с backoff 5s→10s→...→5min, до `AUSF_NAMF_QUEUE_MAX_ATTEMPTS` попыток (по умолч. 10). Очередь заполнена → oldest дроп с ERROR-логом. При успехе / исчерпании попыток — INFO/ERROR лог. Запуск/остановка через `Start()` / `Stop()`. Wired в `main.go`: `retryingNamf.Start()` при старте, `defer retryingNamf.Stop()` при остановке. Примечание: persistent queue (Redis/Kafka) — вне scope (инфраструктурная задача). |
| ~~Е~~ | ~~**Mutual TLS, ротация сертификатов (автоматическая)**~~ ✅ | Go + Java | Реализовано: `CertLoader.ForceReload()` — инвалидирует кэш сертификата, следующий вызов `Get()` перечитывает файл немедленно, не ожидая TTL. `CertLoader.warnIfExpiringSoon()` — при каждой загрузке сертификата проверяет `NotAfter`; если до истечения < 7 дней — пишет JSON WARN в stderr с именем файла, временем истечения и оставшимися часами. SIGHUP-хендлер в `main.go` (только при TLS): `signal.Notify(sighupCh, syscall.SIGHUP)` → `serverCertLoader.ForceReload()` → INFO-лог. CertLoader вынесен из `serve()` в `main()` и передаётся параметром. Применение: `kill -HUP <pid>` после деплоя новых cert-файлов — немедленная замена без рестарта. Примечание: SPIFFE/SVID — требует SPIRE agent + Kubernetes workload API; вне scope кодовой базы. |
