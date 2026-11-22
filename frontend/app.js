import { ApiError, MoviePickerApi } from './api.js';

const $ = (selector) => document.querySelector(selector);
const dom = {
  filters: $('#filters-form'), sidebar: $('#filters-panel'), backdrop: $('#drawer-backdrop'),
  toggle: $('#open-filters'), count: $('#result-count'), query: $('#query'),
  form: $('#chat-form'), send: $('#send-button'), error: $('#form-error'),
  workspace: $('#workspace'), welcome: $('#welcome'), suggestions: $('#suggestions'),
  conversation: $('#conversation'), messages: $('#messages'), activeFilters: $('#active-filters'),
  about: $('#about-dialog'),
  genres: $('#genre-options'), useLLM: $('#use-llm'), auth: $('#auth-dialog'),
  authOpen: $('#auth-open'), authLogout: $('#auth-logout'), account: $('#account'),
  accountEmail: $('#account-email'), authForm: $('#auth-form'), authEmail: $('#auth-email'),
  authPassword: $('#auth-password'), authError: $('#auth-error'), authSubmit: $('#auth-submit'),
  authMode: $('#auth-mode'), authClose: $('#auth-close'),
};

const genreNames = {
  'Science Fiction': 'Фантастика', Drama: 'Драма', Comedy: 'Комедия',
  Thriller: 'Триллер', Adventure: 'Приключения', Romance: 'Мелодрама',
  Crime: 'Криминал', Animation: 'Мультфильм', Fantasy: 'Фэнтези',
  Horror: 'Ужасы', Family: 'Семейный', Action: 'Боевик', Mystery: 'Детектив',
  War: 'Военный', History: 'Исторический', Music: 'Музыкальный', Western: 'Вестерн',
};

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.classList.add('icon');
  svg.setAttribute('aria-hidden', 'true');
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
  use.setAttribute('href', '#i-' + name);
  svg.append(use);
  return svg;
}

const client = new MoviePickerApi();
let account = null;
let catalog = null;
let catalogLoading = null;
let authBusy = true;
let authMode = 'login';
let pending = null;
let requestVersion = 0;
let lastDrawerFocus = null;
const formatRating = (value) => value.toLocaleString('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
function readFilters() {
  const form = new FormData(dom.filters);
  return {
    genre_ids: form.getAll('genre_ids').map(Number),
    year_from: form.get('year_from') === '' ? null : Number(form.get('year_from')),
    year_to: form.get('year_to') === '' ? null : Number(form.get('year_to')),
    min_rating: Number(form.get('rating')) || null,
  };
}

function filterLabels(filters, count) {
  const labels = (catalog?.genres || [])
    .filter((genre) => filters.genre_ids.includes(genre.id))
    .map((genre) => genreNames[genre.name] || genre.name);
  if (filters.year_from !== null && filters.year_to !== null) labels.push(filters.year_from + '–' + filters.year_to);
  else if (filters.year_from !== null) labels.push('от ' + filters.year_from + ' года');
  else if (filters.year_to !== null) labels.push('до ' + filters.year_to + ' года');
  if (filters.min_rating) labels.push('рейтинг ' + filters.min_rating + '+');
  if (count !== undefined) labels.push('до ' + count + ' фильмов');
  return labels;
}

function showCatalog(data) {
  catalog = data;
  dom.genres.replaceChildren();
  if (!data) {
    dom.genres.append(element('span', 'field-note', account ? 'Загрузка жанров…' : 'Жанры доступны после входа.'));
    updateFilters();
    return;
  }
  for (const genre of data.genres) {
    const label = element('label', 'genre-chip');
    const input = element('input');
    input.type = 'checkbox';
    input.name = 'genre_ids';
    input.value = genre.id;
    label.append(input, element('span', '', genreNames[genre.name] || genre.name));
    dom.genres.append(label);
  }
  for (const id of ['year-from', 'year-to']) {
    const field = $('#' + id);
    field.min = data.year_from ?? 1800;
    field.max = data.year_to ?? 3000;
  }
  dom.count.max = data.max_results || 50;
  dom.count.defaultValue = data.default_results || 10;
  dom.count.value = dom.count.defaultValue;
  updateFilters();
}

function loadCatalog() {
  if (catalogLoading) return catalogLoading;
  const requestingAccount = account;
  const loading = client.catalog().then((data) => {
    if (account !== requestingAccount) return false;
    showCatalog(data);
    showError('');
    return true;
  }).catch((error) => {
    if (account !== requestingAccount) return false;
    if (error.status === 401) {
      sessionExpired();
    } else {
      dom.genres.replaceChildren(element('span', 'field-note', 'Каталог недоступен.'));
      showError(errorText(error, 'Каталог недоступен. Повторите попытку.'));
    }
    return false;
  }).finally(() => {
    if (catalogLoading === loading) catalogLoading = null;
  });
  catalogLoading = loading;
  return loading;
}

function updateFilters() {
  const filters = readFilters();
  const labels = filterLabels(filters);
  dom.activeFilters.replaceChildren();
  for (const label of labels) dom.activeFilters.append(element('span', 'active-filter', label));
  dom.activeFilters.hidden = labels.length === 0;
  $('#filter-count').textContent = String(labels.length);
  $('#filter-count').hidden = labels.length === 0;
}

function showError(message) {
  dom.error.textContent = message;
  dom.error.hidden = !message;
}

function setBusy(busy) {
  dom.send.disabled = busy || !dom.query.value.trim();
  dom.query.readOnly = busy;
  dom.query.setAttribute('aria-busy', String(busy));
  dom.send.setAttribute('aria-label', busy ? 'Подбираем фильмы' : 'Подобрать фильмы');
}

function errorText(error, fallback) {
  return error instanceof ApiError ? error.message : fallback;
}

function setAccount(user) {
  account = user;
  catalogLoading = null;
  dom.account.hidden = !user;
  dom.accountEmail.textContent = user?.email || '';
  dom.authOpen.hidden = Boolean(user);
  showCatalog(null);
}

function showAuthError(message = '') {
  dom.authError.textContent = message;
  dom.authError.hidden = !message;
}

function setAuthBusy(value) {
  authBusy = value;
  for (const button of [dom.authOpen, dom.authLogout, dom.authSubmit, dom.authMode, dom.authClose]) {
    button.disabled = value;
  }
}

function setAuthMode(mode) {
  authMode = mode;
  const registering = mode === 'register';
  $('#auth-title').textContent = registering ? 'Регистрация' : 'Вход';
  dom.authSubmit.textContent = registering ? 'Зарегистрироваться' : 'Войти';
  dom.authMode.textContent = registering ? 'У меня есть аккаунт' : 'Создать аккаунт';
  dom.authPassword.autocomplete = registering ? 'new-password' : 'current-password';
  dom.authPassword.minLength = registering ? 8 : 1;
  dom.authPassword.value = '';
  showAuthError();
}

function openAuth(message) {
  setAuthMode('login');
  if (message) showAuthError(message);
  if (!dom.auth.open) dom.auth.showModal();
  dom.authEmail.focus();
}

function sessionExpired() {
  setAccount(null);
  resetConversation({ keepQuery: true, focus: false });
  openAuth('Сессия истекла. Войдите снова.');
}

async function authenticate(event) {
  event.preventDefault();
  if (authBusy) return;
  setAuthBusy(true);
  showAuthError();
  try {
    const user = await client.authenticate(authMode, { email: dom.authEmail.value.trim(), password: dom.authPassword.value });
    dom.authForm.reset();
    dom.auth.close();
    setAccount(user);
    loadCatalog();
  } catch (error) {
    showAuthError(errorText(error, 'Не удалось связаться с сервером.'));
  } finally {
    setAuthBusy(false);
  }
}

async function logout() {
  if (authBusy) return;
  setAuthBusy(true);
  showError('');
  try {
    await client.logout();
    setAccount(null);
    resetConversation({ focus: false });
  } catch (error) {
    showError(errorText(error, 'Не удалось выйти. Попробуйте ещё раз.'));
  } finally {
    setAuthBusy(false);
    if (!account) dom.authOpen.focus();
  }
}

async function restoreSession() {
  try {
    const user = await client.restore();
    if (user) {
      setAccount(user);
      loadCatalog();
    }
  } catch (error) {
    showError(errorText(error, 'Не удалось проверить сессию. Попробуйте войти.'));
  } finally {
    setAuthBusy(false);
  }
}

function enterConversation() {
  dom.welcome.hidden = true;
  dom.suggestions.hidden = true;
  dom.conversation.hidden = false;
  dom.workspace.classList.add('has-messages');
}

function scrollToLatest() {
  dom.conversation.scrollTop = dom.conversation.scrollHeight;
}

function assistantMessage() {
  const message = element('section', 'assistant-message');
  message.setAttribute('aria-label', 'Ответ Movie Picker');
  const heading = element('div', 'assistant-heading');
  heading.append(element('span', 'assistant-mark', 'm'), element('span', '', 'Movie Picker'));
  message.append(heading);
  return message;
}

function renderMovie(movie, index) {
  const card = element('article', 'movie-card');
  const content = element('div', 'movie-content');
  const top = element('div', 'movie-topline');
  const rating = element('span', 'movie-rating');
  top.append(element('h3', '', movie.title));
  if (movie.rating != null) {
    rating.append(icon('star'), document.createTextNode(formatRating(movie.rating)));
    top.append(rating);
  }
  const meta = [
    movie.year,
    movie.genres.map((genre) => genreNames[genre.name] || genre.name).join(' / '),
  ].filter(Boolean).join(' · ');
  const details = element('details', 'movie-details');
  const overview = element('p', 'movie-overview', movie.overview);
  overview.lang = 'en';
  details.append(element('summary', '', 'О фильме · описание на английском'), overview);
  content.append(top);
  if (meta) content.append(element('p', 'movie-meta', meta));
  if (movie.reason) content.append(element('p', 'movie-reason', movie.reason));
  if (movie.overview) content.append(details);
  card.append(element('span', 'movie-number', String(index + 1).padStart(2, '0')), content);
  return card;
}

function renderResponse(response) {
  const message = assistantMessage();
  if (!response.items.length) {
    message.append(element('p', 'assistant-text', 'Подходящих фильмов не найдено. Попробуйте уточнить запрос или смягчить фильтры.'));
  } else {
    message.append(element('p', 'assistant-text', response.mode === 'generated' ? 'Подборка с объяснениями LLM' : 'Результаты поиска'));
    const list = element('div', 'movie-list');
    response.items.forEach((movie, index) => list.append(renderMovie(movie, index)));
    message.append(list);
  }
  for (const warning of response.warnings || []) {
    if (warning.code !== 'NO_MATCHES') message.append(element('p', 'result-note', warning.message));
  }
  return message;
}

function validateRequest() {
  const filters = readFilters();
  const yearsReversed = filters.year_from !== null && filters.year_to !== null && filters.year_from > filters.year_to;
  $('#year-from').setCustomValidity(yearsReversed ? 'Начальный год не должен быть больше конечного.' : '');
  if (!dom.filters.checkValidity()) {
    if (matchMedia('(max-width: 900px)').matches) setDrawer(true);
    dom.filters.reportValidity();
    return false;
  }
  return true;
}

async function submitQuery() {
  const query = dom.query.value.trim();
  if (!query || pending || authBusy) return;
  if (!account) {
    openAuth('Войдите или зарегистрируйтесь, чтобы получить подборку.');
    return;
  }
  if (!catalog && !(await loadCatalog())) return;
  if (pending || !account || dom.query.value.trim() !== query) return;
  if (!validateRequest()) return;
  showError('');
  const request = {
    query, filters: readFilters(), limit: Number(dom.count.value),
    generate_explanations: dom.useLLM.checked, disable_llm: !dom.useLLM.checked,
  };
  const version = ++requestVersion;
  const controller = new AbortController();
  pending = controller;
  setBusy(true);
  setDrawer(false);
  enterConversation();

  const user = element('div', 'user-message', query);
  user.append(element('span', 'message-filters', filterLabels(request.filters, request.limit).join(' · ')));
  dom.messages.append(user);
  const loading = assistantMessage();
  const dots = element('div', 'thinking');
  dots.setAttribute('role', 'status');
  dots.setAttribute('aria-label', 'Подбираем фильмы');
  dots.append(element('span'), element('span'), element('span'), element('div', 'thinking-label', 'Ищем ваш следующий фильм…'));
  loading.append(dots);
  dom.messages.append(loading);
  scrollToLatest();

  try {
    const response = await client.recommend(request, { signal: controller.signal });
    if (version !== requestVersion) return;
    const result = renderResponse(response);
    loading.replaceWith(result);
    dom.query.value = '';
    dom.query.style.height = '';
    setBusy(false);
    const firstResult = result.getBoundingClientRect();
    const scrollParent = dom.conversation.getBoundingClientRect();
    dom.conversation.scrollTop += firstResult.top - scrollParent.top - 12;
    dom.query.focus({ preventScroll: true });
    // Announce a compact result, not all movie descriptions, to screen readers.
    status.textContent = response.items.length ? 'Подбор готов. Фильмов: ' + response.items.length + '.' : 'Фильмы не найдены.';
  } catch (error) {
    if (error.name !== 'AbortError' && version === requestVersion) {
      if (error.status === 401) sessionExpired();
      else {
        loading.remove();
        showError(errorText(error, 'Не удалось завершить подбор. Попробуйте ещё раз.'));
      }
    }
  } finally {
    if (version === requestVersion) {
      pending = null;
      setBusy(false);
    }
  }
}

function resetConversation({ keepQuery = false, focus = true } = {}) {
  requestVersion += 1;
  pending?.abort();
  pending = null;
  dom.messages.replaceChildren();
  dom.workspace.classList.remove('has-messages');
  dom.conversation.hidden = true;
  dom.welcome.hidden = false;
  dom.suggestions.hidden = false;
  if (!keepQuery) {
    dom.query.value = '';
    dom.query.style.height = '';
  }
  status.textContent = '';
  setBusy(false);
  showError('');
  if (focus) dom.query.focus({ preventScroll: true });
}

function setDrawer(open) {
  const mobile = matchMedia('(max-width: 900px)').matches;
  dom.sidebar.inert = mobile && !open;
  dom.workspace.inert = mobile && open;
  $('.site-header').inert = mobile && open;
  if (open && mobile && !dom.sidebar.classList.contains('is-open')) lastDrawerFocus = document.activeElement;
  dom.sidebar.classList.toggle('is-open', open && mobile);
  dom.backdrop.hidden = !(open && mobile);
  dom.toggle.setAttribute('aria-expanded', String(open && mobile));
  if (mobile && open) {
    dom.sidebar.setAttribute('role', 'dialog');
    dom.sidebar.setAttribute('aria-modal', 'true');
    $('#close-filters').focus();
  } else {
    dom.sidebar.removeAttribute('role');
    dom.sidebar.removeAttribute('aria-modal');
    if (lastDrawerFocus && dom.sidebar.contains(document.activeElement)) lastDrawerFocus.focus();
    lastDrawerFocus = null;
  }
}

function openAbout() { dom.about.showModal(); }
function closeAbout() { dom.about.close(); }

dom.form.addEventListener('submit', (event) => {
  event.preventDefault();
  submitQuery();
});
dom.query.addEventListener('input', () => {
  dom.query.style.height = 'auto';
  dom.query.style.height = Math.min(134, dom.query.scrollHeight) + 'px';
  setBusy(Boolean(pending));
});
dom.query.addEventListener('keydown', (event) => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    dom.form.requestSubmit();
  }
});
dom.filters.addEventListener('submit', (event) => event.preventDefault());
dom.filters.addEventListener('input', () => { $('#year-from').setCustomValidity(''); updateFilters(); showError(''); });
$('#reset-filters').addEventListener('click', () => {
  dom.filters.reset();
  $('#year-from').setCustomValidity('');
  updateFilters();
  showError('');
});
$('#count-minus').addEventListener('click', () => {
  dom.count.value = String(Math.max(1, (Number(dom.count.value) || 10) - 1));
});
$('#count-plus').addEventListener('click', () => {
  dom.count.value = String(Math.min(Number(dom.count.max), (Number(dom.count.value) || 10) + 1));
});
$('#new-chat').addEventListener('click', () => resetConversation());
document.querySelectorAll('[data-prompt]').forEach((button) => button.addEventListener('click', () => {
  dom.query.value = button.dataset.prompt;
  dom.query.dispatchEvent(new Event('input'));
  dom.form.requestSubmit();
}));
$('#open-filters').addEventListener('click', () => setDrawer(true));
$('#close-filters').addEventListener('click', () => setDrawer(false));
dom.backdrop.addEventListener('click', () => setDrawer(false));
$('#apply-filters').addEventListener('click', () => {
  if (validateRequest()) setDrawer(false);
});
dom.sidebar.addEventListener('keydown', (event) => {
  if (!dom.sidebar.classList.contains('is-open')) return;
  if (event.key === 'Escape') { event.preventDefault(); setDrawer(false); }
  if (event.key === 'Tab') {
    const targets = [...dom.sidebar.querySelectorAll('button, input')].filter((node) => !node.disabled && node.getClientRects().length);
    const first = targets[0];
    const last = targets[targets.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }
});
matchMedia('(max-width: 900px)').addEventListener('change', () => setDrawer(false));
document.querySelectorAll('[data-about]').forEach((button) => button.addEventListener('click', openAbout));
$('#close-about').addEventListener('click', closeAbout);
$('#start-picking').addEventListener('click', () => { closeAbout(); dom.query.focus(); });
dom.about.addEventListener('click', (event) => {
  if (event.target !== dom.about) return;
  const box = dom.about.getBoundingClientRect();
  if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) closeAbout();
});
const status = element('div', 'sr-only');
status.setAttribute('role', 'status');
status.setAttribute('aria-live', 'polite');
document.body.append(status);
dom.authOpen.addEventListener('click', () => openAuth());
dom.authLogout.addEventListener('click', logout);
dom.authForm.addEventListener('submit', authenticate);
dom.authMode.addEventListener('click', () => setAuthMode(authMode === 'login' ? 'register' : 'login'));
dom.authClose.addEventListener('click', () => dom.auth.close());
dom.auth.addEventListener('cancel', (event) => { if (authBusy) event.preventDefault(); });
dom.auth.addEventListener('close', () => { dom.authPassword.value = ''; });
updateFilters();
setDrawer(false);
restoreSession();
