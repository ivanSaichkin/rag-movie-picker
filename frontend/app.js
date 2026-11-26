import { ApiError, MoviePickerApi } from './api.js?v=4';

const $ = (selector, root = document) => root.querySelector(selector);
const dom = {
  shell: $('#app-shell'), workspace: $('#workspace'), title: $('#workspace-title'), chatStage: $('#chat-stage'),
  sidebarHeader: $('#sidebar-header'), workspaceHeader: $('#workspace-header'),
  messages: $('#messages'), welcome: $('#welcome'), welcomeNote: $('#welcome-note'),
  chatNav: $('#chat-nav'), chatList: $('#chat-list'), chatsNote: $('#chats-note'),
  chatsToggle: $('#chats-toggle'), sidebarToggle: $('#sidebar-toggle'),
  chatsClose: $('#chats-close'), chatDrawer: $('#chat-drawer'), chatScrim: $('#chat-scrim'),
  composer: $('#chat-form'), composerArea: $('#composer-area'),
  dock: $('#composer-dock'), query: $('#query'), send: $('#send-button'), error: $('#form-error'),
  genres: $('#genre-options'), yearFrom: $('#year-from'), yearTo: $('#year-to'),
  rating: $('#rating'), limit: $('#result-count'), strategy: $('#strategy'),
  auth: $('#auth-dialog'), authOpen: $('#auth-open'), authForm: $('#auth-form'),
  authEmail: $('#auth-email'), authPassword: $('#auth-password'), authSubmit: $('#auth-submit'),
  authError: $('#auth-error'), authClose: $('#auth-close'), passwordToggle: $('#password-toggle'),
  removeDialog: $('#remove-dialog'),
  profile: $('#profile'), logout: $('#auth-logout'), collectionLink: $('#my-movies'),
  collection: $('#collection'), tabs: $('#collection-tabs'), collectionPanel: $('#collection-panel'),
  collectionDisplay: $('#collection-display'), collectionList: $('#collection-list'),
  collectionDetail: $('#collection-detail'),
  collectionFeedback: $('#collection-feedback'), pagination: $('#collection-pagination'),
  previousPage: $('#collection-prev'), nextPage: $('#collection-next'), status: $('#status'),
  theme: $('#theme-toggle'), mobileThemeSlot: $('#mobile-theme-slot'),
};
const mobileLayout = matchMedia('(max-width: 720px)');

const genreNames = {
  'Science Fiction': 'Фантастика', Drama: 'Драма', Comedy: 'Комедия',
  Thriller: 'Триллер', Adventure: 'Приключения', Romance: 'Мелодрама',
  Crime: 'Криминал', Animation: 'Мультфильм', Fantasy: 'Фэнтези',
  Horror: 'Ужасы', Family: 'Семейный', Action: 'Боевик', Mystery: 'Детектив',
  War: 'Военный', History: 'Исторический', Music: 'Музыкальный', Western: 'Вестерн',
  Documentary: 'Документальный', 'TV Movie': 'Телефильм', Foreign: 'Зарубежный',
};
const sceneGenre = {
  'Science Fiction': 'space', Mystery: 'mystery', Crime: 'mystery', Thriller: 'mystery',
  Horror: 'mystery', Comedy: 'comedy', Family: 'comedy', Animation: 'comedy',
  Action: 'adventure', Adventure: 'adventure', Fantasy: 'adventure', Western: 'adventure',
  Drama: 'drama', Romance: 'drama', History: 'drama',
};
const genreScenes = {
  space: ['observatory', 'moon', 'orbit', 'hangar'],
  mystery: ['street', 'corridor'],
  comedy: ['diner', 'records'],
  adventure: ['mountain', 'road'],
  drama: ['station', 'records'],
  archive: ['reel', 'station', 'road', 'records'],
};
const strategyNames = { bm25: 'По словам', transformer: 'По смыслу', hybrid: 'Гибридный поиск' };
const wanted = 'want_to_watch';
const watched = 'watched';
const statuses = [wanted, watched];
const pageSize = 20;
const formatNumber = new Intl.NumberFormat('ru-RU');
const formatRating = new Intl.NumberFormat('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const formatDate = new Intl.DateTimeFormat('ru-RU', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const voteForm = new Intl.PluralRules('ru-RU');
const client = new MoviePickerApi();
let account = null;
let session = new AbortController();
let authBusy = true;
let authAfterSubmit = false;
let catalog = null;
let catalogLoading = null;
let limitEdited = false;
let chats = [];
let activeChat = createChat();
let collectionOpen = false;
let collectionStatus = wanted;
let selectedMovie = null;
let pendingRemoval = null;
const lists = new Map();
const movieStates = new Map();
const movieUpdates = new Set();

function element(tag, className, label) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (label !== undefined) node.textContent = label;
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

function button(label, className = 'button', iconName, iconOnly = false) {
  const node = element('button', className);
  node.type = 'button';
  if (iconName) node.append(icon(iconName));
  if (iconOnly) {
    node.setAttribute('aria-label', label);
    node.title = label;
  } else {
    node.append(document.createTextNode(label));
  }
  return node;
}

function setMessage(node, message = '') {
  node.textContent = message;
  node.hidden = !message;
}

function errorText(error, fallback) {
  return error instanceof ApiError ? error.message : fallback;
}

function hidePopovers(except = null) {
  document.querySelectorAll('.popover[open]').forEach((popover) => {
    if (popover !== except) popover.open = false;
  });
}

function createChat() {
  return { id: null, loaded: true, loading: false, title: '', date: null, draft: '', scroll: 0, pending: false, transcript: element('div', 'transcript') };
}

function renderChatList() {
  dom.chatList.replaceChildren();
  dom.chatsNote.hidden = chats.length > 0;
  dom.chatsNote.textContent = account
    ? 'Здесь появятся ваши сохранённые чаты.'
    : 'Гостевые чаты доступны до перезагрузки страницы.';
  for (const chat of chats) {
    const item = element('li');
    const tab = button('', 'chat-link');
    const time = element('time', '', formatDate.format(chat.date));
    time.dateTime = chat.date.toISOString();
    tab.append(element('span', 'chat-name', chat.title), time);
    if (!collectionOpen && activeChat === chat) tab.setAttribute('aria-current', 'true');
    tab.addEventListener('click', () => switchChat(chat, { focus: !mobileLayout.matches }));
    item.append(tab);
    dom.chatList.append(item);
  }
}

function markActiveChat() {
  const fragment = account && activeChat.id ? '#chat=' + encodeURIComponent(activeChat.id) : '';
  history.replaceState(null, '', location.pathname + location.search + fragment);
}

async function loadChatList(initialChat, restoreId) {
  const { signal } = session;
  try {
    const data = await client.listChats({ signal });
    if (signal.aborted) return;
    const known = new Set(chats.map(chat => chat.id));
    for (const summary of data.items) {
      if (known.has(summary.chat_id)) continue;
      const chat = createChat();
      Object.assign(chat, { id: summary.chat_id, loaded: false, title: summary.title, date: new Date(summary.created_at) });
      chats.push(chat);
    }
    renderChatList();
    if (activeChat === initialChat && !initialChat.date && !initialChat.draft && !collectionOpen) {
      const previous = chats.find(chat => chat.id === restoreId);
      if (previous) switchChat(previous, { focus: false });
    }
  } catch (error) {
    if (signal.aborted) return;
    if (error.status === 401) return sessionExpired();
    dom.chatsNote.hidden = false;
    const retry = button('Повторить', 'button');
    retry.addEventListener('click', () => loadChatList(initialChat, restoreId), { once: true });
    dom.chatsNote.replaceChildren(document.createTextNode('Не удалось загрузить историю. '), retry);
  }
}

async function loadChatTurns(chat) {
  if (chat.loaded || chat.loading) return;
  const { signal } = session;
  chat.loading = true;
  chat.transcript.replaceChildren(element('p', 'muted', 'Загружаем чат…'));
  try {
    const detail = await client.getChat(chat.id, { signal });
    if (signal.aborted) return;
    const turns = detail.turns.map(entry => {
      const turn = element('div', 'chat-turn');
      turn.append(element('p', 'user-message', entry.query), renderResponse(entry.response));
      return turn;
    });
    chat.transcript.replaceChildren(...turns);
    chat.loaded = true;
    if (turns.length) for (const status of statuses) if (!lists.has(status)) loadMoviePage(status, 1);
  } catch (error) {
    if (signal.aborted) return;
    if (error.status === 401) return sessionExpired();
    const retry = button('Повторить', 'button');
    retry.addEventListener('click', () => loadChatTurns(chat), { once: true });
    chat.transcript.replaceChildren(element('p', 'form-error', 'Не удалось открыть чат.'), retry);
  } finally {
    chat.loading = false;
    if (!signal.aborted && activeChat === chat) renderView();
  }
}

function renderView() {
  dom.title.textContent = collectionOpen ? 'Мои фильмы' : 'Киноархив';
  dom.collectionLink.setAttribute('aria-pressed', String(collectionOpen));
  dom.collection.hidden = !collectionOpen;
  dom.chatStage.hidden = collectionOpen;
  dom.composerArea.hidden = collectionOpen;
  dom.dock.classList.toggle('collection-dock', collectionOpen);
  dom.welcome.hidden = activeChat.transcript.childElementCount > 0;
  updateComposer();
}

function resizeQuery() {
  dom.query.style.height = '';
  if (dom.query.value) dom.query.style.height = Math.min(132, dom.query.scrollHeight) + 'px';
}

function updateComposer() {
  dom.query.readOnly = activeChat.pending;
  dom.send.disabled = authBusy || activeChat.pending || !activeChat.loaded || !dom.query.value.trim();
  const label = activeChat.pending ? 'Подбираем фильмы…' : 'Подобрать фильмы';
  dom.send.setAttribute('aria-label', label);
  dom.send.title = label;
  dom.send.replaceChildren(activeChat.pending ? element('span', 'spinner') : icon('up'));
  dom.query.setAttribute('aria-busy', String(activeChat.pending));
  resizeQuery();
}

function switchChat(chat, { focus = true } = {}) {
  if (!collectionOpen) activeChat.scroll = dom.chatStage.scrollTop;
  collectionOpen = false;
  activeChat = chat;
  markActiveChat();
  if (!chat.loaded) loadChatTurns(chat);
  if (activeChat.transcript.parentNode !== dom.messages) dom.messages.replaceChildren(chat.transcript);
  dom.query.value = chat.draft;
  setMessage(dom.error);
  hidePopovers();
  setMobileChats(false);
  renderView();
  renderChatList();
  refreshRecommendationActions();
  dom.chatStage.scrollTop = chat.scroll;
  if (focus) dom.query.focus({ preventScroll: true });
}

function readFilters() {
  const form = new FormData(dom.composer);
  const readNumber = (name) => form.get(name) === '' ? null : Number(form.get(name));
  return {
    genre_ids: form.getAll('genre_ids').map(Number),
    year_from: readNumber('year_from'),
    year_to: readNumber('year_to'),
    min_rating: readNumber('rating'),
  };
}

function showFilterLabels() {
  const filters = readFilters();
  $('#genre-label').textContent = filters.genre_ids.length ? 'Жанры · ' + filters.genre_ids.length : 'Жанры';
  $('#year-label').textContent = filters.year_from && filters.year_to
    ? filters.year_from + '–' + filters.year_to
    : filters.year_from ? 'От ' + filters.year_from : filters.year_to ? 'До ' + filters.year_to : 'Годы';
  $('#rating-label').textContent = filters.min_rating ? 'Рейтинг ' + formatNumber.format(filters.min_rating) + '+' : 'Рейтинг';
  $('#count-label').textContent = dom.limit.value ? 'До ' + dom.limit.value : 'Лимит';
  $('#strategy-label').textContent = 'Поиск';
  $('#mode-label').textContent = dom.composer.elements.llm.value === 'on' ? 'С LLM' : 'Без LLM';
}

function showCatalog(data) {
  catalog = data;
  const checked = readFilters().genre_ids;
  dom.genres.replaceChildren();
  for (const genre of data?.genres || []) {
    const label = element('label', 'genre-option');
    const control = element('input');
    control.type = 'checkbox';
    control.name = 'genre_ids';
    control.value = genre.id;
    control.checked = checked.includes(genre.id);
    label.append(control, element('span', '', genreNames[genre.name] || genre.name));
    dom.genres.append(label);
  }
  if (!dom.genres.childElementCount) {
    const note = data ? 'Жанры отсутствуют в каталоге.' : 'Загружаем жанры…';
    dom.genres.append(element('p', 'muted', note));
  }
  for (const control of [dom.yearFrom, dom.yearTo]) {
    control.min = data?.year_from ?? 1800;
    control.max = data?.year_to ?? 3000;
  }
  dom.limit.max = data?.max_results || 50;
  dom.limit.defaultValue = data?.default_results || 10;
  if (!limitEdited) dom.limit.value = dom.limit.defaultValue;
  const strategies = data?.available_strategies || [];
  dom.strategy.replaceChildren();
  for (const name of strategies) {
    const option = element('option', '', strategyNames[name] || name);
    option.value = name;
    dom.strategy.append(option);
  }
  if (strategies.includes(data?.default_strategy)) dom.strategy.value = data.default_strategy;
  $('#strategy-filter').hidden = strategies.length < 2;
  showFilterLabels();
}

function loadCatalog() {
  if (catalogLoading) return catalogLoading;
  const { signal } = session;
  const loading = client.catalog({ signal, guest: !account }).then((data) => {
    if (signal.aborted) return false;
    showCatalog(data);
    return true;
  }).catch((error) => {
    if (signal.aborted) return false;
    if (error.status === 401) {
      sessionExpired();
    } else {
      const retry = button('Повторить загрузку', 'button');
      retry.addEventListener('click', () => loadCatalog());
      dom.genres.replaceChildren(element('p', 'muted', 'Каталог недоступен.'), retry);
      setMessage(dom.error, errorText(error, 'Не удалось загрузить каталог. Попробуйте ещё раз.'));
    }
    return false;
  }).finally(() => {
    if (catalogLoading === loading) catalogLoading = null;
  });
  catalogLoading = loading;
  return loading;
}

function validateFilters() {
  const filters = readFilters();
  const reversed = filters.year_from !== null && filters.year_to !== null && filters.year_from > filters.year_to;
  dom.yearTo.setCustomValidity(reversed ? 'Конечный год должен быть не меньше начального.' : '');
  const invalid = [dom.yearFrom, dom.yearTo, dom.rating, dom.limit].find((input) => !input.validity.valid);
  if (!invalid) return true;
  const popover = invalid.closest('.popover');
  hidePopovers(popover);
  popover.open = true;
  invalid.reportValidity();
  return false;
}

function setAccount(user, { keepGuestDraft = false, restoreId = null } = {}) {
  session.abort();
  session = new AbortController();
  const guestDraft = keepGuestDraft ? activeChat.draft : '';
  if (!keepGuestDraft) {
    dom.composer.reset();
    dom.yearTo.setCustomValidity('');
    limitEdited = false;
  }
  account = user;
  chats = [];
  catalogLoading = null;
  lists.clear();
  movieStates.clear();
  movieUpdates.clear();
  collectionStatus = wanted;
  selectedMovie = null;
  pendingRemoval = null;
  if (dom.removeDialog.open) dom.removeDialog.close();
  $('#remove-description').textContent = '';
  dom.status.textContent = '';
  dom.collectionList.replaceChildren();
  dom.collectionDetail.replaceChildren();
  dom.collectionFeedback.replaceChildren();
  dom.pagination.hidden = true;
  dom.profile.hidden = !user;
  dom.authOpen.hidden = Boolean(user);
  dom.collectionLink.hidden = !user;
  $('#account-email').textContent = user?.email || '';
  $('#account-initial').textContent = user?.email?.charAt(0).toUpperCase() || '';
  dom.welcomeNote.textContent = user
    ? 'Расскажите о сюжете, настроении или жанре.'
    : 'Ищите фильмы без входа. Войдите, чтобы сохранять чаты и получать пояснения.';
  $('input[name=llm][value=on]').closest('label').querySelector('small').textContent = user ? 'Отбор и пояснения' : 'Доступно после входа';
  const freshChat = createChat();
  freshChat.draft = guestDraft;
  showCatalog(null);
  switchChat(freshChat, { focus: false });
  if (user) loadChatList(freshChat, restoreId);
  loadCatalog();
}

function setAuthBusy(busy) {
  authBusy = busy;
  for (const control of [dom.authOpen, dom.authSubmit, dom.authClose, dom.logout, dom.authEmail, dom.authPassword, dom.passwordToggle]) {
    control.disabled = busy;
  }
  dom.authForm.setAttribute('aria-busy', String(busy));
  dom.authSubmit.textContent = busy ? 'Входим…' : authAfterSubmit ? 'Войти и подобрать' : 'Войти';
  updateComposer();
}

function setPasswordVisible(visible) {
  dom.authPassword.type = visible ? 'text' : 'password';
  dom.passwordToggle.setAttribute('aria-pressed', String(visible));
  dom.passwordToggle.setAttribute('aria-label', visible ? 'Скрыть пароль' : 'Показать пароль');
  dom.passwordToggle.replaceChildren(icon(visible ? 'eye-off' : 'eye'));
}

function authFieldError(input, message = '') {
  setMessage($('#' + input.id + '-error'), message);
  if (message) input.setAttribute('aria-invalid', 'true');
  else input.removeAttribute('aria-invalid');
}

function openAuth({ afterSubmit = false, message = '' } = {}) {
  if (authBusy || dom.auth.open) return;
  authAfterSubmit = afterSubmit;
  dom.authPassword.value = '';
  setPasswordVisible(false);
  authFieldError(dom.authEmail);
  authFieldError(dom.authPassword);
  setMessage(dom.authError, message);
  dom.authSubmit.textContent = afterSubmit ? 'Войти и подобрать' : 'Войти';
  dom.auth.showModal();
  dom.authEmail.focus();
}

function sessionExpired() {
  client.clearSession();
  setAccount(null);
  openAuth({ message: 'Сессия завершилась. Войдите снова.' });
}

async function authenticate(event) {
  event.preventDefault();
  if (authBusy) return;
  setMessage(dom.authError);
  dom.authEmail.value = dom.authEmail.value.trim();
  const emailError = !dom.authEmail.value ? 'Введите почту.' : !dom.authEmail.validity.valid ? 'Проверьте адрес почты.' : '';
  const passwordError = dom.authPassword.value ? '' : 'Введите пароль.';
  authFieldError(dom.authEmail, emailError);
  authFieldError(dom.authPassword, passwordError);
  if (emailError || passwordError) {
    (emailError ? dom.authEmail : dom.authPassword).focus();
    return;
  }
  const shouldRecommend = authAfterSubmit;
  setAuthBusy(true);
  let user;
  try {
    user = await client.login({ email: dom.authEmail.value, password: dom.authPassword.value });
    dom.authForm.reset();
    dom.auth.close();
    setAccount(user, { keepGuestDraft: true });
  } catch (error) {
    setMessage(dom.authError, errorText(error, 'Не удалось связаться с сервером.'));
  } finally {
    setAuthBusy(false);
  }
  if (user) {
    dom.query.focus({ preventScroll: true });
    if (shouldRecommend) submitQuery();
  }
}

async function logout() {
  if (authBusy) return;
  setAuthBusy(true);
  setMessage(dom.error);
  try {
    await client.logout();
    setAccount(null);
  } catch (error) {
    setMessage(dom.error, errorText(error, 'Не удалось выйти. Попробуйте ещё раз.'));
  } finally {
    setAuthBusy(false);
    if (!account) dom.authOpen.focus();
  }
}

async function restoreSession() {
  try {
    const user = await client.restore();
    setAccount(user, { keepGuestDraft: true, restoreId: new URLSearchParams(location.hash.slice(1)).get('chat') });
  } catch (error) {
    setMessage(dom.error, errorText(error, 'Не удалось проверить сессию. Попробуйте войти.'));
  } finally {
    setAuthBusy(false);
  }
}

function movieGenres(movie) {
  return (movie.genres || []).map((genre) => genreNames[genre.name] || genre.name).join(', ');
}

function movieMetadata(movie, includeRating = false) {
  const metadata = [movie.year, movieGenres(movie)];
  if (includeRating && movie.rating != null) metadata.push('TMDB ' + formatRating.format(movie.rating));
  if (includeRating && movie.vote_count != null) {
    const votes = { one: 'голос', few: 'голоса', many: 'голосов', other: 'голоса' };
    metadata.push(formatNumber.format(movie.vote_count) + ' ' + votes[voteForm.select(movie.vote_count)]);
  }
  return metadata.filter(Boolean).join(' · ');
}

function archiveFrame(movie, usedScenes) {
  const frame = element('span', 'archive-frame');
  const index = Math.abs(Number(movie.movie_id) || 0);
  const genres = (movie.genres || []).map(({ name }) => sceneGenre[name]).filter(Boolean);
  const primary = genres.find((genre) => genre !== 'drama') || genres[0] || 'archive';
  const scenes = genreScenes[primary];
  const offset = index % scenes.length;
  const preferred = scenes[offset];
  const candidates = [...scenes.slice(offset), ...scenes.slice(0, offset), ...genreScenes.archive];
  const scene = usedScenes ? candidates.find((name) => !usedScenes.has(name)) || preferred : preferred;
  usedScenes?.add(scene);
  frame.dataset.scene = scene;
  frame.setAttribute('aria-hidden', 'true');
  frame.append(movie.year ? element('span', 'frame-year', String(movie.year)) : icon('film'));
  return frame;
}

function dvdCase(movie, className, usedScenes) {
  const card = button('', 'dvd-case ' + className);
  card.dataset.movieId = movie.movie_id;
  card.setAttribute('aria-label', movie.title);
  card.title = movie.title;
  const body = element('span', 'dvd-case-body');
  body.setAttribute('aria-hidden', 'true');
  const front = element('span', 'dvd-case-front');
  const fallback = archiveFrame(movie, usedScenes);
  fallback.prepend(element('span', 'dvd-cover-title', movie.title));
  front.append(fallback, element('span', 'dvd-poster'));
  const spine = element('span', 'dvd-case-spine');
  spine.append(element('span', 'dvd-spine-title', movie.title));
  body.append(front, spine);
  card.append(body);
  return card;
}

function bindCaseSelection(card, select) {
  card.addEventListener('pointermove', (event) => {
    if (event.pointerType === 'mouse' && !event.buttons) select(false);
  });
  card.addEventListener('click', () => select(true));
}

function revealCase(shelf, card) {
  const outer = shelf.getBoundingClientRect();
  const inner = card.getBoundingClientRect();
  const width = $('.dvd-case-body', card).offsetWidth;
  const left = inner.left - outer.left - 8;
  const right = inner.left + width - outer.right + 8;
  if (left < 0) shelf.scrollBy({ left });
  else if (right > 0) shelf.scrollBy({ left: right });
}

function describeMovie(movie, headingLevel, { explain = false } = {}) {
  const content = element('div', 'movie-description');
  const title = element('h' + headingLevel, 'movie-title', movie.title);
  content.append(title);
  const metadata = movieMetadata(movie, true);
  if (metadata) content.append(element('p', 'movie-meta', metadata));
  const overview = element('section', 'movie-overview');
  overview.append(
    element('h' + (headingLevel + 1), '', 'О фильме'),
    element('p', '', movie.overview?.trim() || 'Описание отсутствует.'),
  );
  content.append(overview);
  const reason = explain && movie.reason?.trim();
  if (reason) {
    const explanation = element('section', 'movie-explanation');
    explanation.append(element('h' + (headingLevel + 1), '', 'Почему в подборке'), element('p', '', reason));
    content.append(explanation);
  }
  return content;
}

function movieAction(movie, action, label, iconName, { iconOnly = false } = {}) {
  const control = button(label, 'button movie-action' + (iconOnly ? ' icon-button' : ''), iconName, iconOnly);
  control.dataset.action = action;
  control.dataset.movieId = movie.movie_id;
  return control;
}

function recommendationActions(movie) {
  const actions = element('div', 'movie-actions recommendation-actions');
  const bookmark = movieAction(movie, 'bookmark', 'Хочу посмотреть', 'bookmark', { iconOnly: true });
  const markWatched = movieAction(movie, 'mark-watched', 'Отметить просмотренным', 'eye', { iconOnly: true });
  bookmark.addEventListener('click', () => updateMovieFromRecommendation(movie, 'bookmark'));
  markWatched.addEventListener('click', () => updateMovieFromRecommendation(movie, 'mark-watched'));
  bookmark.append(element('span', 'recommendation-action-label'));
  markWatched.append(element('span', 'recommendation-action-label'));
  actions.append(bookmark, markWatched);
  syncRecommendationActions(actions);
  return actions;
}

function syncRecommendationActions(actions) {
  const bookmark = $('[data-action="bookmark"]', actions);
  const markWatched = $('[data-action="mark-watched"]', actions);
  const movieId = Number(bookmark.dataset.movieId);
  const state = movieStates.get(movieId)?.status;
  const busy = movieUpdates.has(movieId);
  const bookmarkLabel = state === wanted ? 'Убрать из «Хочу посмотреть»' : 'Хочу посмотреть';
  const watchedLabel = state === watched ? 'Просмотрено' : 'Отметить просмотренным';
  bookmark.setAttribute('aria-pressed', String(state === wanted));
  markWatched.setAttribute('aria-pressed', String(state === watched));
  for (const [control, label] of [[bookmark, bookmarkLabel], [markWatched, watchedLabel]]) {
    control.setAttribute('aria-label', label);
    control.title = label;
    $('.recommendation-action-label', control).textContent = label;
    control.setAttribute('aria-disabled', String(busy || state === watched));
  }
}

function refreshRecommendationActions() {
  dom.messages.querySelectorAll('.recommendation-actions').forEach(syncRecommendationActions);
}

function renderResponse(response) {
  const items = response.items || [];
  const result = element('section', 'recommendation');
  result.setAttribute('aria-label', 'Ответ Movie Picker');
  if (response.mode) result.dataset.mode = response.mode;
  if (response.strategy_used) result.dataset.strategyUsed = response.strategy_used;
  result.append(element('h2', 'sr-only', 'Подборка фильмов'));

  const warnings = (response.warnings || []).filter((warning) => warning.message);
  if (warnings.length) {
    const notes = element('ul', 'result-notes');
    for (const warning of warnings) {
      const note = element('li', '', warning.message);
      if (warning.code) note.dataset.warningCode = warning.code;
      notes.append(note);
    }
    result.append(notes);
  }
  if (!items.length) {
    if (!warnings.some((warning) => warning.code === 'NO_MATCHES')) {
      result.append(element('p', 'muted empty-results', 'Подходящих фильмов не найдено.'));
    }
    return result;
  }

  const controls = element('div', 'strip-controls');
  const previous = button('Предыдущий фильм', 'button icon-button', 'left', true);
  const next = button('Следующий фильм', 'button icon-button', 'right', true);
  controls.append(previous, next);

  const stage = element('div', 'recommendation-shelf dvd-display');
  const lamp = element('span', 'dvd-lamp');
  lamp.setAttribute('aria-hidden', 'true');
  const strip = element('div', 'filmstrip dvd-shelf');
  strip.setAttribute('role', 'group');
  strip.setAttribute('aria-label', 'Фильмы в подборке');
  const details = element('div', 'recommendation-details');
  const usedScenes = new Set();
  const frames = items.map((movie, index) => {
    const card = dvdCase(movie, 'filmstrip-frame', usedScenes);
    bindCaseSelection(card, (scroll) => select(index, { scroll }));
    strip.append(card);
    return card;
  });
  let selected = -1;
  function select(index, { focus = false, scroll = false } = {}) {
    if (index < 0 || index >= frames.length) return;
    const frame = frames[index];
    if (selected !== index) {
      selected = index;
      for (const [position, card] of frames.entries()) {
        card.setAttribute('aria-pressed', String(position === index));
        card.tabIndex = position === index ? 0 : -1;
      }
      previous.setAttribute('aria-disabled', String(index === 0));
      next.setAttribute('aria-disabled', String(index === frames.length - 1));
      details.replaceChildren(describeMovie(items[index], 3, { explain: true }), recommendationActions(items[index]));
    }
    if (focus) frame.focus({ preventScroll: true });
    if (scroll) revealCase(strip, frame);
  }

  strip.addEventListener('keydown', (event) => {
    const target = { ArrowLeft: selected - 1, ArrowRight: selected + 1, Home: 0, End: frames.length - 1 }[event.key];
    if (target === undefined) return;
    event.preventDefault();
    select(target, { focus: true, scroll: true });
  });
  previous.addEventListener('click', () => select(selected - 1, { scroll: true }));
  next.addEventListener('click', () => select(selected + 1, { scroll: true }));
  select(0);
  stage.append(lamp, controls, strip);
  result.append(stage, details);
  return result;
}

function scrollToTurn(turn) {
  dom.chatStage.scrollTop += turn.getBoundingClientRect().top - dom.chatStage.getBoundingClientRect().top - 12;
}

async function submitQuery() {
  const chat = activeChat;
  const query = chat.draft.trim();
  if (!query || authBusy || chat.pending || !chat.loaded || collectionOpen) return;
  if (!account && dom.composer.elements.llm.value === 'on') {
    openAuth({ afterSubmit: true });
    return;
  }

  const { signal } = session;
  chat.pending = true;
  setMessage(dom.error);
  updateComposer();
  let turn;
  try {
    if (!catalog && !(await loadCatalog())) return;
    if (signal.aborted || chat !== activeChat || collectionOpen || !validateFilters()) return;
    const useLLM = dom.composer.elements.llm.value === 'on';
    const request = {
      query, filters: readFilters(), limit: Number(dom.limit.value),
      disable_llm: !useLLM, generate_explanations: useLLM,
    };
    if (account && chat.id) request.chat_id = chat.id;
    if (dom.strategy.value) request.strategy = dom.strategy.value;
    hidePopovers();
    if (mobileLayout.matches) dom.query.blur();
    if (!chat.date) {
      chat.title = [...query].slice(0, 80).join('');
      chat.date = new Date();
      chats.unshift(chat);
      renderChatList();
    }
    turn = element('div', 'chat-turn');
    const prompt = element('p', 'user-message', query);
    const loading = element('p', 'turn-status');
    loading.setAttribute('role', 'status');
    loading.append(element('span', 'spinner'), document.createTextNode('Подбираем фильмы…'));
    turn.append(prompt, loading);
    chat.transcript.append(turn);
    renderView();
    scrollToTurn(turn);
    const response = await client.recommend(request, { signal, guest: !account });
    if (signal.aborted) return;
    if (account && response.chat_id) {
      if (!chat.id) chat.title = [...query.trim().replace(/\s+/g, ' ')].slice(0, 80).join('');
      chat.id = response.chat_id;
      chats = [chat, ...chats.filter(other => other !== chat && other.id !== chat.id)];
      renderChatList();
      if (chat === activeChat) renderView();
      if (chat === activeChat) markActiveChat();
    }
    const result = renderResponse(response);
    loading.replaceWith(result);
    chat.draft = '';

    if (chat === activeChat) {
      dom.query.value = '';
      if (!collectionOpen) scrollToTurn(turn);
      dom.status.textContent = response.items.length ? 'Подборка готова. Фильмов: ' + response.items.length + '.' : 'Фильмы не найдены.';
    }
    if (account && response.items.length) {
      for (const status of statuses) if (!lists.has(status)) loadMoviePage(status, 1);
    }
  } catch (error) {
    if (signal.aborted || error.name === 'AbortError') return;
    if (error.status === 401) {
      sessionExpired();
      return;
    }
    const message = errorText(error, 'Не удалось завершить подбор. Попробуйте ещё раз.');
    const loading = turn && $('.turn-status', turn);
    if (loading) {
      loading.replaceWith(element('p', 'turn-status form-error', message));
      if (chat === activeChat && !collectionOpen) dom.status.textContent = message;
    } else {
      setMessage(dom.error, message);
    }
  } finally {
    chat.pending = false;
    if (!signal.aborted && chat === activeChat) updateComposer();
  }
}

function renderCollectionTabs() {
  for (const tab of dom.tabs.querySelectorAll('[role="tab"]')) {
    const current = tab.dataset.status === collectionStatus;
    const total = lists.get(tab.dataset.status)?.data?.total;
    tab.tabIndex = current ? 0 : -1;
    tab.setAttribute('aria-selected', String(current));
    $('.tab-count', tab).textContent = total == null ? '' : formatNumber.format(total);
    if (current) dom.collectionPanel.setAttribute('aria-labelledby', tab.id);
  }
}

function renderCollectionList() {
  const state = lists.get(collectionStatus);
  const data = state?.data;
  const focus = document.activeElement?.closest('.collection-row');
  const focusedId = focus?.dataset.movieId;
  const scrollLeft = dom.collectionList.scrollLeft;
  let replacement;
  dom.collectionList.replaceChildren();
  dom.collectionFeedback.replaceChildren();
  dom.collectionList.setAttribute('aria-busy', String(Boolean(state?.loading)));
  dom.collectionDisplay.hidden = !data?.items.length;
  if (state?.error) {
    const message = element('p', 'form-error', state.error);
    message.setAttribute('role', 'alert');
    const retry = button('Повторить');
    retry.addEventListener('click', () => loadMoviePage(collectionStatus, state.requested));
    dom.collectionFeedback.append(message, retry);
  } else if (state?.loading && !data) {
    const loading = element('p', 'muted', 'Загружаем список…');
    loading.setAttribute('role', 'status');
    dom.collectionFeedback.append(loading);
  } else if (data && !data.items.length && !state?.loading) {
    dom.collectionFeedback.append(element('p', 'muted', 'В списке пока нет фильмов.'));
  }

  const usedScenes = new Set();
  for (const movie of data?.items || []) {
    const row = dvdCase(movie, 'collection-row', usedScenes);
    row.setAttribute('aria-pressed', String(selectedMovie?.movie_id === movie.movie_id));
    bindCaseSelection(row, (scroll) => {
      if (selectedMovie?.movie_id !== movie.movie_id) {
        selectedMovie = movieStates.get(movie.movie_id) || movie;
        for (const candidate of dom.collectionList.children) {
          candidate.setAttribute('aria-pressed', String(candidate === row));
        }
        setMessage(dom.error);
        renderCollectionDetail();
      }
      if (scroll) revealCase(dom.collectionList, row);
    });
    dom.collectionList.append(row);
    if (row.dataset.movieId === focusedId) replacement = row;
  }
  dom.collectionList.scrollTo({ left: scrollLeft, behavior: 'instant' });
  if (replacement) replacement.focus({ preventScroll: true });

  const pages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 0;
  dom.pagination.hidden = pages <= 1;
  $('#collection-page').textContent = data ? data.page + ' / ' + pages : '';
  dom.previousPage.disabled = !data || data.page <= 1 || state.loading;
  dom.nextPage.disabled = !data || data.page >= pages || state.loading;
}

function showCollectionActionState() {
  for (const control of dom.collectionDetail.querySelectorAll('button[data-action]')) {
    const busy = movieUpdates.has(Number(control.dataset.movieId));
    control.setAttribute('aria-disabled', String(busy || control.dataset.action === 'clear-rating' && selectedMovie?.liked == null));
  }
}

function collectionActions(movie) {
  const actions = element('div', 'movie-actions collection-actions');
  const menu = element('details', 'popover movie-menu');
  const menuToggle = element('summary', 'button icon-button');
  menuToggle.setAttribute('aria-label', 'Другие действия');
  menuToggle.title = 'Другие действия';
  menuToggle.append(icon('more'));
  const panel = element('div', 'popover-panel movie-menu-panel');
  function add(container, action, label, iconName, change) {
    const control = movieAction(movie, action, label, iconName);
    control.addEventListener('click', () => {
      if (control.getAttribute('aria-disabled') === 'true') return;
      if (action === 'remove') {
        pendingRemoval = movie;
        $('#remove-description').textContent = movie.title;
        dom.removeDialog.returnValue = '';
        dom.removeDialog.showModal();
        $('#remove-cancel').focus();
      } else {
        updateCollectionMovie(movie, change);
      }
    });
    container.append(control);
    return control;
  }

  if (movie.status === wanted) {
    add(actions, 'mark-watched', 'Отметить просмотренным', 'eye', { status: watched, liked: null });
  } else {
    const like = add(actions, 'like', 'Понравилось', 'like', { status: watched, liked: true });
    const dislike = add(actions, 'dislike', 'Не понравилось', 'dislike', { status: watched, liked: false });
    like.setAttribute('aria-pressed', String(movie.liked === true));
    dislike.setAttribute('aria-pressed', String(movie.liked === false));
    add(panel, 'move-to-wanted', 'Вернуть в «Хочу посмотреть»', 'return', { status: wanted, liked: null });
    add(panel, 'clear-rating', 'Снять оценку', 'close', { status: watched, liked: null });
  }
  add(panel, 'remove', 'Убрать из списка', 'trash', { status: null, liked: null });
  menu.append(menuToggle, panel);
  actions.append(menu);
  return actions;
}

function renderCollectionDetail({ focusAction = '' } = {}) {
  if (!selectedMovie) {
    const placeholder = element('div', 'collection-placeholder');
    placeholder.append(icon('film'), element('p', '', 'Выберите фильм'));
    dom.collectionDetail.replaceChildren(placeholder);
    return;
  }
  const article = element('article', 'collection-movie');
  article.dataset.movieId = selectedMovie.movie_id;
  article.append(describeMovie(selectedMovie, 2), collectionActions(selectedMovie));
  dom.collectionDetail.replaceChildren(article);
  showCollectionActionState();
  if (focusAction) {
    const next = $('[data-action="' + focusAction + '"]', article);
    const inMenu = next?.closest('.movie-menu');
    (inMenu ? $('summary', inMenu) : next)?.focus({ preventScroll: true });
  }
}

function loadMoviePage(status, page, { force = false } = {}) {
  const prior = lists.get(status);
  if (prior?.loading && prior.requested === page && !force) return;
  const state = { data: prior?.data || null, loading: true, requested: page, error: '' };
  const { signal } = session;
  lists.set(status, state);
  if (collectionOpen && status === collectionStatus) renderCollectionList();

  client.listMovies(status, page, pageSize, { signal }).then((data) => {
    if (signal.aborted || lists.get(status) !== state) return;
    const lastPage = Math.max(1, Math.ceil(data.total / data.page_size));
    if (data.page > lastPage) {
      loadMoviePage(status, lastPage);
      return;
    }
    state.data = data;
    for (const item of data.items) movieStates.set(item.movie_id, item);
    refreshRecommendationActions();
  }).catch((error) => {
    if (signal.aborted || lists.get(status) !== state) return;
    if (error.status === 401) sessionExpired();
    else state.error = errorText(error, 'Не удалось загрузить список.');
  }).finally(() => {
    if (signal.aborted || lists.get(status) !== state) return;
    state.loading = false;
    if (collectionOpen) {
      renderCollectionTabs();
      if (status === collectionStatus) renderCollectionList();
    }
  });
}

function switchCollectionTab(status, { focus = false } = {}) {
  if (status !== collectionStatus) {
    collectionStatus = status;
    selectedMovie = null;
    dom.collectionList.scrollTo({ left: 0, behavior: 'instant' });
    dom.collectionPanel.scrollTop = 0;
    setMessage(dom.error);
    hidePopovers();
    renderCollectionTabs();
    renderCollectionDetail();
    renderCollectionList();
    if (!lists.get(status)?.data) loadMoviePage(status, 1);
  }
  if (focus) $('[aria-selected="true"]', dom.tabs).focus();
}

function openCollection() {
  if (!account || authBusy) return;
  if (collectionOpen) {
    switchChat(activeChat);
    return;
  }
  activeChat.scroll = dom.chatStage.scrollTop;
  collectionOpen = true;
  collectionStatus = wanted;
  selectedMovie = null;
  dom.collectionList.scrollTo({ left: 0, behavior: 'instant' });
  dom.collectionPanel.scrollTop = 0;
  setMessage(dom.error);
  hidePopovers();
  setMobileChats(false);
  renderView();
  renderChatList();
  renderCollectionTabs();
  renderCollectionDetail();
  for (const status of statuses) loadMoviePage(status, 1, { force: true });
}

function turnCollectionPage(offset) {
  const page = lists.get(collectionStatus)?.data?.page;
  if (page == null) return;
  selectedMovie = null;
  dom.collectionList.scrollTo({ left: 0, behavior: 'instant' });
  dom.collectionPanel.scrollTop = 0;
  dom.collectionPanel.focus({ preventScroll: true });
  renderCollectionDetail();
  loadMoviePage(collectionStatus, page + offset);
}

async function lookupMovieStatus(movieId, signal) {
  if (movieStates.has(movieId)) return movieStates.get(movieId);
  for (const status of statuses) {
    let page = 1;
    let total;
    do {
      const data = await client.listMovies(status, page, 100, { signal });
      const match = data.items.find((item) => item.movie_id === movieId);
      if (match) {
        movieStates.set(movieId, match);
        return match;
      }
      page = data.page + 1;
      total = Math.ceil(data.total / data.page_size);
    } while (page <= total);
  }
  return null;
}

function refreshListsAfterUpdate(movieId, previous, updated) {
  for (const status of statuses) {
    const state = lists.get(status);
    const data = state?.data;
    if (data) {
      const oldItems = data.items.filter((item) => item.movie_id !== movieId);
      if (previous?.status === status) data.total = Math.max(0, data.total - 1);
      if (updated?.status === status) {
        data.total += 1;
        if (data.page === 1) oldItems.unshift(updated);
      }
      data.items = oldItems.slice(0, data.page_size);
    }
    loadMoviePage(status, state?.requested || data?.page || 1, { force: true });
  }
  if (collectionOpen) renderCollectionTabs();
}

async function updateMovie(movie, resolve) {
  const id = movie.movie_id;
  if (!account || authBusy || movieUpdates.has(id)) return;
  const { signal } = session;
  movieUpdates.add(id);
  refreshRecommendationActions();
  showCollectionActionState();
  setMessage(dom.error);
  try {
    const { previous, change } = await resolve(signal);
    if (signal.aborted || !change || previous?.status === change.status && previous.liked === change.liked) return;
    const updated = change.status
      ? await client.saveMovie(id, change.status, change.liked, { signal })
      : await client.removeMovie(id, { signal }).catch((error) => {
        if (error.status !== 404) throw error;
        return null;
      });
    if (signal.aborted) return;
    movieStates.set(id, updated);
    if (selectedMovie?.movie_id === id) {
      const focused = document.activeElement;
      const focusAction = dom.collectionDetail.contains(focused) ? focused.dataset.action : '';
      selectedMovie = updated?.status === collectionStatus ? updated : null;
      renderCollectionDetail({ focusAction });
      if (!selectedMovie && focusAction) {
        $('[aria-selected="true"]', dom.tabs).focus({ preventScroll: true });
      }
    }
    refreshListsAfterUpdate(id, previous, updated);
    dom.status.textContent = updated ? 'Список фильмов обновлён.' : 'Фильм удалён из списка.';
  } catch (error) {
    if (signal.aborted || error.name === 'AbortError') return;
    if (error.status === 401) sessionExpired();
    else setMessage(dom.error, errorText(error, 'Не удалось обновить список. Попробуйте ещё раз.'));
  } finally {
    if (!signal.aborted) {
      movieUpdates.delete(id);
      refreshRecommendationActions();
      showCollectionActionState();
    }
  }
}

function updateCollectionMovie(movie, change) {
  updateMovie(movie, () => ({ previous: movieStates.get(movie.movie_id) || movie, change }));
}

function updateMovieFromRecommendation(movie, action) {
  if (!account) {
    openAuth({ message: 'Войдите, чтобы сохранить фильм в личном списке.' });
    return;
  }
  const removeBookmark = action === 'bookmark' && movieStates.get(movie.movie_id)?.status === wanted;
  updateMovie(movie, async (signal) => {
    const previous = await lookupMovieStatus(movie.movie_id, signal);
    if (previous?.status === watched) return { previous, change: null };
    if (removeBookmark && previous?.status !== wanted) return { previous, change: null };
    const status = action === 'mark-watched' ? watched : removeBookmark ? null : wanted;
    return { previous, change: { status, liked: null } };
  });
}

function applyTheme(theme) {
  const light = theme === 'light';
  document.documentElement.dataset.theme = light ? 'light' : 'dark';
  $('meta[name="theme-color"]').content = light ? '#f6f3ed' : '#11110f';
  const label = light ? 'Включить тёмную тему' : 'Включить светлую тему';
  dom.theme.setAttribute('aria-label', label);
  dom.theme.title = label;
  dom.theme.replaceChildren(icon(light ? 'moon' : 'sun'));
}

function initTheme() {
  let theme = 'dark';
  try { theme = localStorage.getItem('movie-picker-theme') || theme; } catch {}
  applyTheme(theme);
  dom.theme.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    applyTheme(next);
    try { localStorage.setItem('movie-picker-theme', next); } catch {}
  });
}

function initSidebar() {
  const storageKey = 'movie-picker-sidebar-collapsed';
  function render(collapsed) {
    dom.shell.classList.toggle('sidebar-collapsed', collapsed);
    dom.sidebarToggle.setAttribute('aria-expanded', String(!collapsed));
    const label = collapsed ? 'Показать историю чатов' : 'Свернуть историю чатов';
    dom.sidebarToggle.setAttribute('aria-label', label);
    dom.sidebarToggle.title = label;
  }
  try { render(localStorage.getItem(storageKey) === 'true'); } catch {}
  dom.sidebarToggle.addEventListener('click', () => {
    const collapsed = dom.sidebarToggle.getAttribute('aria-expanded') === 'true';
    render(collapsed);
    try { localStorage.setItem(storageKey, String(collapsed)); } catch {}
  });
}

function setMobileChats(open, { restoreFocus = true } = {}) {
  const wasOpen = dom.shell.classList.contains('chats-open');
  if (open === wasOpen || (open && !mobileLayout.matches)) return;

  dom.shell.classList.toggle('chats-open', open);
  dom.chatsToggle.setAttribute('aria-expanded', String(open));
  dom.sidebarHeader.inert = open;
  dom.workspace.inert = open;
  dom.chatDrawer.inert = !open && mobileLayout.matches;
  if (open) {
    hidePopovers();
    dom.chatDrawer.setAttribute('role', 'dialog');
    dom.chatDrawer.setAttribute('aria-modal', 'true');
    dom.chatDrawer.setAttribute('aria-labelledby', 'chat-drawer-title');
    dom.chatsClose.focus({ preventScroll: true });
  } else {
    dom.chatDrawer.removeAttribute('role');
    dom.chatDrawer.removeAttribute('aria-modal');
    dom.chatDrawer.removeAttribute('aria-labelledby');
    if (restoreFocus && mobileLayout.matches) dom.chatsToggle.focus({ preventScroll: true });
  }
}

function initChatGestures() {
  let gesture = null;
  document.addEventListener('pointerdown', (event) => {
    if (!mobileLayout.matches || event.pointerType !== 'touch' || !event.isPrimary) return;
    const open = dom.shell.classList.contains('chats-open');
    const surface = event.target.closest('#chat-drawer, #sidebar-header, #workspace-header');
    if (!surface || open !== (surface === dom.chatDrawer)) return;
    if (!open && event.target.closest('button, a, summary')) return;
    gesture = { id: event.pointerId, x: event.clientX, y: event.clientY, open, dragging: false };
  });
  document.addEventListener('pointermove', (event) => {
    if (event.pointerId !== gesture?.id || !gesture.open) return;
    const dx = event.clientX - gesture.x;
    if (!gesture.dragging && dx < -12 && -dx > Math.abs(event.clientY - gesture.y) * 1.35) {
      gesture.dragging = true;
      dom.chatDrawer.setPointerCapture(event.pointerId);
      dom.shell.classList.add('chats-dragging');
    }
    if (gesture.dragging) {
      const travel = Math.max(-dom.chatDrawer.offsetWidth, Math.min(0, dx));
      dom.chatDrawer.style.setProperty('--drawer-drag', travel + 'px');
    }
  });
  function finish(event) {
    if (event.pointerId !== gesture?.id) return;
    const { x, y, open } = gesture;
    gesture = null;
    const dx = event.clientX - x;
    const dy = event.clientY - y;
    const left = dx < -55;
    const rightFromEdge = !open && x < 48 && dx > 55;
    dom.shell.classList.remove('chats-dragging');
    if (event.type === 'pointerup' && (left || rightFromEdge) && Math.abs(dx) > Math.abs(dy) * 1.35) {
      setMobileChats(!open);
    }
    dom.chatDrawer.style.removeProperty('--drawer-drag');
  }
  document.addEventListener('pointerup', finish);
  document.addEventListener('pointercancel', finish);
}

function initMobileLayout() {
  const viewport = window.visualViewport;
  const style = document.documentElement.style;
  let frame = 0;

  function measure() {
    frame = 0;
    if (!mobileLayout.matches) return;
    dom.shell.classList.toggle('is-writing', document.activeElement === dom.query);
    // Zoom belongs to the browser. Keyboard and browser chrome resize at scale 1.
    if (viewport && Math.abs(viewport.scale - 1) > 0.01) return;
    style.setProperty('--mobile-height', Math.round(viewport?.height ?? innerHeight) + 'px');
    style.setProperty('--mobile-top', Math.round(viewport?.offsetTop ?? 0) + 'px');
  }
  function schedule() {
    if (!frame) frame = requestAnimationFrame(measure);
  }
  function moveControls() {
    const themeFocused = document.activeElement === dom.theme;
    if (mobileLayout.matches) {
      const focusWasInDrawer = dom.chatDrawer.contains(document.activeElement);
      dom.mobileThemeSlot.append(dom.theme);
      dom.chatDrawer.inert = !dom.shell.classList.contains('chats-open');
      if (focusWasInDrawer && dom.chatDrawer.inert) dom.chatsToggle.focus({ preventScroll: true });
    } else {
      const focusWasInDrawer = dom.chatDrawer.contains(document.activeElement);
      setMobileChats(false, { restoreFocus: false });
      dom.chatDrawer.inert = false;
      if (focusWasInDrawer) dom.sidebarToggle.focus({ preventScroll: true });
      dom.dock.insertBefore(dom.theme, dom.error);
    }
    if (themeFocused) dom.theme.focus({ preventScroll: true });
    schedule();
  }

  mobileLayout.addEventListener('change', moveControls);
  window.addEventListener('resize', schedule);
  viewport?.addEventListener('resize', schedule);
  viewport?.addEventListener('scroll', schedule);
  document.addEventListener('focusin', schedule);
  document.addEventListener('focusout', schedule);
  moveControls();
  initChatGestures();
}

dom.composer.addEventListener('submit', (event) => {
  event.preventDefault();
  submitQuery();
});
dom.composer.addEventListener('input', (event) => {
  if (event.target === dom.query) {
    activeChat.draft = dom.query.value;
    updateComposer();
  } else {
    if (event.target === dom.limit) limitEdited = true;
    dom.yearTo.setCustomValidity('');
    showFilterLabels();
  }
  setMessage(dom.error);
});
dom.query.addEventListener('keydown', (event) => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    dom.composer.requestSubmit();
  }
});
dom.composer.addEventListener('change', (event) => {
  showFilterLabels();
  const popover = event.target.closest('.mode-popover');
  if (popover) {
    popover.open = false;
    $('summary', popover).focus();
  }
});
for (const suggestion of document.querySelectorAll('[data-prompt]')) {
  suggestion.addEventListener('click', () => {
    activeChat.draft = suggestion.dataset.prompt;
    dom.query.value = activeChat.draft;
    dom.query.focus();
    updateComposer();
  });
}

$('#new-chat').addEventListener('click', () => switchChat(createChat()));
$('#header-new-chat').addEventListener('click', () => switchChat(createChat()));
$('.brand').addEventListener('click', (event) => {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  switchChat(createChat());
});
dom.chatsToggle.addEventListener('click', () => setMobileChats(true));
dom.chatsClose.addEventListener('click', () => setMobileChats(false));
dom.chatScrim.addEventListener('click', () => setMobileChats(false));
dom.chatDrawer.addEventListener('keydown', (event) => {
  if (event.key !== 'Tab' || !dom.shell.classList.contains('chats-open')) return;
  const buttons = [...dom.chatDrawer.querySelectorAll('button:not(:disabled)')].filter((button) => button.getClientRects().length);
  const boundary = buttons[event.shiftKey ? 0 : buttons.length - 1];
  if (event.target !== boundary) return;
  event.preventDefault();
  buttons[event.shiftKey ? buttons.length - 1 : 0].focus();
});
dom.collectionLink.addEventListener('click', openCollection);
dom.tabs.addEventListener('click', (event) => {
  const tab = event.target.closest('[role="tab"]');
  if (tab) switchCollectionTab(tab.dataset.status);
});
dom.tabs.addEventListener('keydown', (event) => {
  const alternate = collectionStatus === wanted ? watched : wanted;
  const direction = { ArrowLeft: alternate, ArrowRight: alternate, Home: wanted, End: watched }[event.key];
  if (!direction) return;
  event.preventDefault();
  switchCollectionTab(direction, { focus: true });
});
dom.previousPage.addEventListener('click', () => turnCollectionPage(-1));
dom.nextPage.addEventListener('click', () => turnCollectionPage(1));
document.addEventListener('click', (event) => hidePopovers(event.target.closest('.popover')));
document.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape' || dom.auth.open || dom.removeDialog.open) return;
  const open = $('.popover[open]');
  if (open) {
    event.preventDefault();
    open.open = false;
    $('summary', open).focus();
  } else if (dom.shell.classList.contains('chats-open')) {
    event.preventDefault();
    setMobileChats(false);
  }
});

dom.authOpen.addEventListener('click', () => openAuth());
dom.logout.addEventListener('click', logout);
dom.authForm.addEventListener('submit', authenticate);
for (const field of [dom.authEmail, dom.authPassword]) {
  field.addEventListener('input', () => {
    authFieldError(field);
    setMessage(dom.authError);
  });
}
dom.passwordToggle.addEventListener('click', () => setPasswordVisible(dom.authPassword.type === 'password'));
dom.authClose.addEventListener('click', () => dom.auth.close());
dom.auth.addEventListener('cancel', (event) => { if (authBusy) event.preventDefault(); });
dom.auth.addEventListener('close', () => {
  dom.authPassword.value = '';
  setPasswordVisible(false);
  authAfterSubmit = false;
});
dom.removeDialog.addEventListener('close', () => {
  const movie = pendingRemoval;
  pendingRemoval = null;
  if (movie && dom.removeDialog.returnValue === 'remove') {
    updateCollectionMovie(movie, { status: null, liked: null });
  }
});

dom.messages.append(activeChat.transcript);
showFilterLabels();
initTheme();
initSidebar();
initMobileLayout();
restoreSession();
