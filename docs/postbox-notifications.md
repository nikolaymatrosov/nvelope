# Уведомления Yandex Cloud Postbox

Уведомление записывается в [поток данных](https://yandex.cloud/ru/docs/data-streams/concepts/glossary#stream-concepts) Yandex Data Streams в формате JSON. Последовательность и набор полей могут отличаться от описанных ниже.

## Типы уведомлений

### Send — приём письма сервисом

Приходит, когда Yandex Cloud Postbox принял письмо в обработку.

```json
{
    "eventType": "Send",
    "mail": {
        "timestamp": "2024-04-25T18:05:04.84108+03:00",
        "messageId": "vgAyRUls8591ybPKeH-Ov",
        "identityId": "nWh0ZpVEgnKO1bghxydXn",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "vgAyRUls8591ybPKeH-Ov",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "send": {},
    "eventId": "vgAyRUls8591ybPKeH-Ov:0"
}
```

### Delivery — доставка письма

Приходит, когда получателю отправили письмо и его почтовый клиент подтвердил приём письма.

```json
{
    "eventType": "Delivery",
    "mail": {
        "timestamp": "2024-04-25T18:05:04.84108+03:00",
        "messageId": "vgAyRUls8591ybPKeH-Ov",
        "identityId": "nWh0ZpVEgnKO1bghxydXn",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "vgAyRUls8591ybPKeH-Ov",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "ses:outgoing-tls-cipher": ["AES_128_GCM_SHA256"],
            "ses:outgoing-tls-version": ["TLSv1.3"],
            "ses:outgoing-ip": ["51.250.56.125"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "bounce": null,
    "delivery": {
        "timestamp": "2024-04-25T18:05:14.84107+03:00",
        "processingTimeMillis": 9999,
        "recipients": ["abc@example.com"]
    },
    "eventId": "ce3uqnS9pzQBMsnaAbrT_:0"
}
```

### Bounce — письмо не доставлено

Приходит, когда почтовый клиент получателя на попытку доставки отвечает ошибкой, которую Yandex Cloud Postbox считает не требующей повторной попытки доставки, или адрес получателя находится в стоп-листе.

```json
{
    "eventType": "Bounce",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "bounce": {
        "bounceType": "Permanent",
        "bounceSubType": "Undetermined",
        "bouncedRecipients": [
            {
                "emailAddress": "abc@example.com",
                "action": "failed",
                "status": "5.7.1",
                "diagnosticCode": "Other"
            }
        ],
        "timestamp": "2024-04-25T18:08:04.973666+03:00"
    },
    "delivery": null,
    "eventId": "jdMtnVniDeHqlQX8ygwEX:0"
}
```

### Open — письмо открыто

Приходит, когда получатель открыл письмо.

```json
{
    "eventType": "Open",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "open": {
        "ipAddress": "192.0.2.1",
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "userAgent": "Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_3 like Mac OS X) AppleWebKit/603.3.8 (KHTML, like Gecko) Mobile/14G60"
    },
    "eventId": "jdMtnVniDeHqlQX8ygwEX:0"
}
```

### Click — переход по ссылке в письме

Приходит, когда получатель перешёл по ссылке в письме.

```json
{
    "eventType": "Click",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "click": {
        "ipAddress": "192.0.2.1",
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "userAgent": "Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_3 like Mac OS X) AppleWebKit/603.3.8 (KHTML, like Gecko) Mobile/14G60",
        "url": "https://example.com/some-link",
        "linkTags": {
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "eventId": "jdMtnVniDeHqlQX8ygwEX:0"
}
```

### DeliveryDelay — доставка задерживается

После того как Yandex Cloud Postbox успешно принял письмо, обычно оно отправляется немедленно. Однако иногда может возникнуть небольшая задержка доставки. В таком случае приходит данное уведомление.

```json
{
    "eventType": "DeliveryDelay",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "deliveryDelay": {
        "delayType": "General",
        "delayedRecipients": [
            {
                "emailAddress": "recipient@example.com"
            }
        ],
        "timestamp": "2024-04-25T18:10:04.973666+03:00"
    },
    "eventId": "jdMtnVniDeHqlQX8ygwEX:0"
}
```

### Unsubscribe — получатель отписался от рассылки

Приходит, когда получатель отписался от рассылки через механизм «отказ от подписки в один клик» (`one-click unsubscribe`), добавленный Yandex Cloud Postbox в письмо.

```json
{
    "eventType": "Unsubscribe",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "subscription": {
        "contactList": "my-list",
        "timestamp": "2024-04-25T18:08:04.973666+03:00",
        "source": "UnsubscribeHeader"
    }
}
```

### Complaint — жалоба на письмо

Приходит, когда получатель пожаловался на полученное письмо, а интернет-провайдер (ISP) отправил эту жалобу в Yandex Cloud Postbox.

```json
{
    "eventType": "Complaint",
    "mail": {
        "timestamp": "2024-04-25T18:08:04.933666+03:00",
        "messageId": "QA_JPkU2fkpIWdkxAOASH",
        "identityId": "ZtYk0rrjN87m-Ovxjte1G",
        "commonHeaders": {
            "from": ["User <user@example.com>"],
            "date": "Thu, 27 Jun 2024 14:05:45 +0000",
            "to": ["Recipient Name <recipient@example.com>"],
            "messageId": "QA_JPkU2fkpIWdkxAOASH",
            "subject": "Message sent using Yandex Cloud Postbox"
        },
        "tags": {
            "ses:configuration-set": ["kXVCt2Vd4dvm3MDvpc5Ml"],
            "ses:from-domain": ["example.com"],
            "ses:source-ip": ["123.123.123.123"],
            "key1": ["value1"],
            "key2": ["value2"]
        }
    },
    "complaint": {
        "complainedRecipients": [
            {
                "emailAddress": "recipient@example.com"
            }
        ],
        "complaintFeedbackType": "abuse",
        "arrivalDate": "2024-04-25T18:05:04.84108+03:00",
        "timestamp": "2024-04-25T18:10:04.973666+03:00",
        "userAgent": "Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_3 like Mac OS X) AppleWebKit/603.3.8 (KHTML, like Gecko) Mobile/14G60"
    },
    "eventId": "jdMtnVniDeHqlQX8ygwEX:0"
}
```

## Формат уведомлений

Все даты — в формате [RFC 3339](https://www.ietf.org/rfc/rfc3339.txt) (`2006-01-02T15:04:05Z07:00`).

### Основной объект

| Поле | Тип | Описание |
| --- | --- | --- |
| `notificationType` | Строка | Тип уведомления. Возможные значения: `Bounce`, `Complaint`, `Delivery`, `Send`. |
| `mail` | [Mail](#объект-mail) | Общая информация об отправленном письме. |
| `bounce` | [Bounce](#объект-bounce) | Информация о том, что письмо не доставлено. Обязателен, если `notificationType` — `Bounce`, иначе отсутствует. |
| `delivery` | [Delivery](#объект-delivery) | Информация о доставке письма отдельному получателю. Обязателен, если `notificationType` — `Delivery`, иначе отсутствует. |
| `complaint` | [Complaint](#объект-complaint) | Информация о жалобе получателя на письмо. Обязателен, если `notificationType` — `Complaint`, иначе отсутствует. |
| `subscription` | [Subscription](#объект-subscription) | Информация о том, что получатель отписался от рассылки. Обязателен, если `notificationType` — `Subscription`, иначе отсутствует. |
| `open` | [Open](#объект-open) | Информация о том, что письмо было открыто. Обязателен, если `notificationType` — `Open`, иначе отсутствует. |
| `eventId` | Строка | Уникальный идентификатор события. |

### Объект Mail

| Поле | Тип | Описание |
| --- | --- | --- |
| `timestamp` | Строка | Время, когда письмо было принято Yandex Cloud Postbox. |
| `messageId` | Строка | Уникальный идентификатор письма. У одного письма может быть несколько получателей. Выдаётся Yandex Cloud Postbox при приёме письма в обработку. |
| `identityId` | Строка | Идентификатор адреса Yandex Cloud Postbox, который используется при отправке письма. |
| `commonHeaders` | [CommonHeaders](#объект-commonheaders) | Основные заголовки письма. |
| `tags` | Объект | Теги, добавленные к письму. |

### Объект CommonHeaders

| Поле | Тип | Описание |
| --- | --- | --- |
| `from` | Массив строк | Содержимое заголовка `From`, разбитое по адресам. |
| `to` | Массив строк | Содержимое заголовка `To`, разбитое по адресам. |
| `subject` | Строка | Содержимое заголовка `Subject`. |
| `date` | Строка | Содержимое заголовка `Date`. |
| `messageId` | Строка | Уникальный идентификатор письма. Выдаётся Yandex Cloud Postbox при приёме письма. |

### Объект Send

Пустой объект.

### Объект Bounce

| Поле | Тип | Описание |
| --- | --- | --- |
| `bounceType` | Строка | Тип ошибки. Возможные значения: `Permanent` — письмо не доставлено. |
| `bounceSubType` | Строка | Подтип ошибки. Возможные значения: `Undetermined` — неизвестная ошибка; `Suppressed` — письмо не доставлено из-за того, что получатель находится в стоп-листе. |
| `bouncedRecipients` | Массив [BounceRecipient](#объект-bouncerecipient) | Информация о получателе письма и связанной с ним ошибке доставки, если она была. |
| `timestamp` | Строка | Время, когда получена ошибка от почтового клиента получателя. |

### Объект BounceRecipient

| Поле | Тип | Описание |
| --- | --- | --- |
| `emailAddress` | Строка | Электронный адрес получателя. |
| `action` | Строка | Необязательное поле. Результат отправки. Возможные значения: `failed`. |
| `status` | Строка | Необязательное поле. SMTP-код ответа. |
| `diagnosticCode` | Строка | Необязательное поле. Расширенный текст ошибки. Может содержать текст ошибки от почтового клиента получателя. |

### Объект Click

| Поле | Тип | Описание |
| --- | --- | --- |
| `ipAddress` | Строка | IP-адрес устройства получателя, с которого перешли по ссылке. |
| `timestamp` | Строка | Время, когда получатель перешёл по ссылке. |
| `userAgent` | Строка | Идентификационная строка (`User-Agent`) устройства или почтового клиента, с которого перешли по ссылке. |
| `url` | Строка | Оригинальный URL, по которому перешёл получатель. |
| `linkTags` | Объект | Теги, добавленные к ссылке. |

### Объект Complaint

| Поле | Тип | Описание |
| --- | --- | --- |
| `complainedRecipients` | Массив [ComplainedRecipient](#объект-complainedrecipient) | Информация о получателях, которые могли подать жалобу. |
| `timestamp` | Строка | Время, когда интернет-провайдер отправил жалобу в Yandex Cloud Postbox. |
| `complaintFeedbackType` | Строка | Необязательное поле. Тип жалобы из отчёта интернет-провайдера. Возможные значения: `abuse` — нежелательная почта или иное злоупотребление; `auth-failure` — ошибка аутентификации письма; `fraud` — фишинг или мошенничество; `not-spam` — получатель не считает письмо спамом (используется для исправления ложного срабатывания); `other` — жалоба, не соответствующая остальным типам; `virus` — в письме обнаружен вирус. |
| `arrivalDate` | Строка | Необязательное поле. Время, когда оригинальное письмо поступило на сервер получателя. |
| `userAgent` | Строка | Необязательное поле. Значение поля `User-Agent` из отчёта о жалобе — имя и версия системы, которая сгенерировала отчёт. |

### Объект ComplainedRecipient

| Поле | Тип | Описание |
| --- | --- | --- |
| `emailAddress` | Строка | Электронный адрес получателя, который мог подать жалобу. |

### Объект Delivery

| Поле | Тип | Описание |
| --- | --- | --- |
| `timestamp` | Строка | Время, когда Yandex Cloud Postbox отправил письмо и получил успешный ответ от почтового клиента получателя. |
| `processingTimeMillis` | Целое число | Время, которое потребовалось на обработку письма, в миллисекундах. |
| `recipients` | Массив строк | Адреса получателей. |

### Объект DeliveryDelay

| Поле | Тип | Описание |
| --- | --- | --- |
| `delayType` | Строка | Тип задержки. Возможные значения: `General`. |
| `delayedRecipients` | Массив [DelayedRecipient](#объект-delayedrecipient) | Информация о получателе письма и связанной с ним задержке доставки. |
| `timestamp` | Строка | Время, когда случилась задержка доставки. |

### Объект DelayedRecipient

| Поле | Тип | Описание |
| --- | --- | --- |
| `emailAddress` | Строка | Электронный адрес получателя. |

### Объект Subscription

| Поле | Тип | Описание |
| --- | --- | --- |
| `contactList` | Строка | Имя списка контактов, с которым связано письмо. |
| `timestamp` | Строка | Время, когда получатель отписался от рассылки. |
| `source` | Строка | Источник отписки. Возможные значения: `UnsubscribeHeader`. |

### Объект Open

| Поле | Тип | Описание |
| --- | --- | --- |
| `ipAddress` | Строка | IP-адрес устройства получателя, с которого было открыто письмо. |
| `timestamp` | Строка | Время, когда письмо было открыто. |
| `userAgent` | Строка | Идентификационная строка (`User-Agent`) устройства или почтового клиента, с которого было открыто письмо. |

## Системные теги

При отправке письма Yandex Cloud Postbox добавляет к письму следующие системные теги, которые затем включаются в уведомления.

### Общие теги

| Тег | Описание |
| --- | --- |
| `ses:configuration-set` | Идентификатор [конфигурации](https://yandex.cloud/ru/docs/postbox/concepts/glossary#configuration), использованной при отправке письма. |
| `ses:from-domain` | Домен, с которого отправлено письмо. |
| `ses:source-ip` | IP-адрес сервера, с которого пользователь отправил письмо в Yandex Cloud Postbox. |

### Дополнительные теги (только в уведомлениях о доставке)

| Тег | Описание |
| --- | --- |
| `ses:outgoing-tls-version` | Версия TLS, использованная при отправке письма на сервер получателя. |
| `ses:outgoing-tls-cipher` | Шифр TLS, использованный при отправке письма на сервер получателя. |
| `ses:outgoing-ip` | IP-адрес сервера, с которого Yandex Cloud Postbox отправил письмо на сервер получателя. |
