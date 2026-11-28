# SalarySense — локальный чат

Чат отвечает на вопросы по 30 документам SalarySense из архива `SalarySense_15_questions_30_documents.zip`. `corpus.json` — текстовая база знаний в UTF-8. Индекс BM25 строится из неё в памяти при старте. После изменения файла перезапустите сервер.

Веб-сервер Python раздаёт страницу со стилями Movie Picker и передаёт вопросы в оригинальный `RagService` из [SalarySense](https://github.com/DemidovCyrill/SalarySense-/tree/backend-rag-bm25). Движок закреплён через Git submodule на `f6d486a`. Он находит до четырёх документов и передаёт их в OpenRouter для ответа со ссылками на источники. Текст каждого источника раскрывается в чате. Без ключа или при сбое OpenRouter чат показывает найденные документы.

## Запуск на Windows

Из корня ветки `ml_salarysense`:

```powershell
git submodule update --init salarysense_demo/engine
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r salarysense_demo/requirements.txt
```

Для ответов модели скопируйте `salarysense_demo/.env.example` в `salarysense_demo/.env` и укажите `OPENROUTER_API_KEY`. Стандартная модель — `openai/gpt-4o-mini`. Её можно поменять через `OPENROUTER_MODEL`. Файл `.env` локальный.

```powershell
.\.venv\Scripts\python.exe -m salarysense_demo
```

Откройте [http://127.0.0.1:8090](http://127.0.0.1:8090). Пример вопроса для проверки — «Что произойдёт с рынком и зарплатами, если я поменяю регион или формат работы?».

Для этого запуска достаточно Python 3.11 или новее. Go, Docker, PostgreSQL, Redis и Qdrant не нужны. Обычный запуск Movie Picker описан в корневом README.
