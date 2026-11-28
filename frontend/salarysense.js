const form = document.querySelector("#chat-form");
const query = document.querySelector("#query");
const messages = document.querySelector("#messages");
const welcome = document.querySelector("#welcome");
const stage = document.querySelector("#chat-stage");
const send = document.querySelector("#send-button");
const newChat = document.querySelector("#new-chat");
const mode = document.querySelector("#mode");
const theme = document.querySelector("#theme-toggle");

async function request(path, options) {
  const response = await fetch(`/api/${path}`, options);
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "Не удалось получить ответ.");
  return data;
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text) node.textContent = text;
  return node;
}

function renderReply(reply, result) {
  reply.replaceChildren(element("p", "", result.answer));
  if (!result.sources.length) return;
  const sources = element("div", "knowledge-sources");
  for (const source of result.sources) {
    const item = element("details", "knowledge-source");
    item.append(element("summary", "", `${source.id} · ${source.title}`), element("p", "", source.fragment));
    sources.append(item);
  }
  reply.append(sources);
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const question = query.value.trim();
  if (!question || send.disabled) return;
  query.value = "";
  welcome.hidden = true;
  const turn = element("article", "chat-turn");
  const reply = element("div", "knowledge-reply", "Ищем ответ в документах…");
  turn.append(element("p", "user-message", question), reply);
  messages.append(turn);
  send.disabled = newChat.disabled = true;
  stage.scrollTop = stage.scrollHeight;
  try {
    const result = await request("ask", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ question }),
    });
    renderReply(reply, result);
    if (result.mode === "retrieval_only") mode.textContent = "BM25";
    if (result.mode === "answered") mode.textContent = "BM25 + LLM";
  } catch (error) {
    reply.textContent = error.message || "Не удалось получить ответ от сервера.";
  } finally {
    send.disabled = newChat.disabled = false;
  }
});

query.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    form.requestSubmit();
  }
});

document.querySelector("#suggestions").addEventListener("click", (event) => {
  const button = event.target.closest("[data-question]");
  if (!button) return;
  query.value = button.dataset.question;
  form.requestSubmit();
});

newChat.addEventListener("click", () => {
  query.value = "";
  messages.replaceChildren();
  welcome.hidden = false;
  query.focus();
});

theme.addEventListener("click", () => {
  const light = document.documentElement.dataset.theme !== "light";
  document.documentElement.dataset.theme = light ? "light" : "dark";
  theme.setAttribute("aria-pressed", String(light));
});

request("status").then((status) => {
  mode.textContent = status.llm_enabled ? "BM25 + LLM" : "BM25";
  document.querySelector("#corpus-note").textContent = `Документов в базе: ${status.documents}`;
}).catch(() => {
  mode.textContent = "Сервер недоступен";
});
