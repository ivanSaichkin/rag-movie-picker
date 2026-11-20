import { catalog } from './catalog.js';

const $ = (selector) => document.querySelector(selector);
const dom = {
  filters: $('#filters-form'), sidebar: $('#filters-panel'), backdrop: $('#drawer-backdrop'),
  toggle: $('#open-filters'), count: $('#result-count'), query: $('#query'),
  form: $('#chat-form'), send: $('#send-button'), error: $('#form-error'),
  workspace: $('#workspace'), welcome: $('#welcome'), suggestions: $('#suggestions'),
  conversation: $('#conversation'), messages: $('#messages'), activeFilters: $('#active-filters'),
  about: $('#about-dialog'),
};

const genreNames = {
  'Science Fiction': 'Фантастика', Drama: 'Драма', Comedy: 'Комедия',
  Thriller: 'Триллер', Adventure: 'Приключения', Romance: 'Мелодрама',
  Crime: 'Криминал', Animation: 'Мультфильм', Fantasy: 'Фэнтези',
  Horror: 'Ужасы', Family: 'Семейный', Action: 'Боевик', Mystery: 'Детектив',
  War: 'Военный', History: 'Исторический', Music: 'Музыкальный', Western: 'Вестерн',
};

// UI demo only. This intentionally does not implement BM25 or imitate an LLM.
// Replace this client with an HTTP adapter when the backend is ready.
const themes = [
  { pattern: /космо|планет|галакт|астронавт|space|planet|galax/i, label: 'космос и другие миры', terms: ['space', 'planet', 'galaxy', 'astronaut', 'interstellar'], ids: [157336, 286217, 49047, 62, 11, 1891, 1892, 118340] },
  { pattern: /уют|добр|тепл|тёпл|спокойн|отдох|cozy|feel.good/i, label: 'уютный вечер', terms: ['friendship', 'family', 'love', 'life'], ids: [13, 1124, 194, 862, 585, 12, 14160, 106646, 10193] },
  { pattern: /детектив|развязк|загад|расслед|mystery|detective|twist/i, label: 'загадки и неожиданные развязки', terms: ['murder', 'mystery', 'detective', 'investigation', 'killer'], ids: [807, 77, 629, 146233, 210577, 11321] },
  { pattern: /фантасти|будущ|science.fiction|sci.fi|future/i, label: 'фантастические истории', terms: ['future', 'science fiction', 'robot', 'artificial intelligence'], genres: ['Science Fiction'] },
  { pattern: /приключ|путешеств|adventure|journey/i, label: 'приключения', terms: ['journey', 'adventure', 'quest', 'travel'], genres: ['Adventure'] },
  { pattern: /комеди|смешн|юмор|посме|comedy|funny/i, label: 'лёгкое настроение', terms: ['comedy', 'funny'], genres: ['Comedy'] },
  { pattern: /любов|романти|мелодрам|romance|romantic/i, label: 'любовные истории', terms: ['love', 'romance', 'romantic'], genres: ['Romance'] },
  { pattern: /триллер|напряж|thriller|suspense/i, label: 'напряжённый сюжет', terms: ['thriller', 'danger', 'killer'], genres: ['Thriller'] },
  { pattern: /семь|семей|реб[её]н|деть|мульт|family|animation|kids/i, label: 'семейный просмотр', terms: ['family', 'friendship', 'animation'], genres: ['Family', 'Animation'] },
  { pattern: /фэнтез|маг|волшеб|fantasy|magic/i, label: 'волшебные миры', terms: ['magic', 'wizard', 'fantasy', 'kingdom'], genres: ['Fantasy'] },
  { pattern: /ужас|страш|хоррор|horror|scary/i, label: 'страшное кино', terms: ['horror', 'terror', 'monster'], genres: ['Horror'] },
  { pattern: /драм|трогат|смысл|серь[её]з|drama|touching/i, label: 'истории, которые трогают', terms: ['drama', 'life'], genres: ['Drama'] },
  { pattern: /кримин|мафи|гангстер|crime|mafia|gangster/i, label: 'криминальные истории', terms: ['crime', 'mafia', 'gangster', 'criminal'], genres: ['Crime'] },
  { pattern: /войн|военн|war|military/i, label: 'военные истории', terms: ['war', 'soldier', 'military'], genres: ['War'] },
  { pattern: /супергер|марвел|marvel|superhero/i, label: 'супергерои', terms: ['superhero', 'super power', 'marvel'], ids: [24428, 1726, 155, 177572] },
  { pattern: /времен|time.travel/i, label: 'путешествия во времени', terms: ['time travel', 'future', 'past'], ids: [105, 157336] },
];

const normalized = (value) => value.toLowerCase().normalize('NFKC');
const searchableCatalog = catalog.map((movie) => ({
  ...movie,
  searchText: normalized([movie.title, movie.overview, ...movie.genres, ...movie.keywords].join(' ')),
}));

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

class DemoRecommendationClient {
  async recommend(request, { signal } = {}) {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(resolve, 650);
      if (signal?.aborted) {
        clearTimeout(timer);
        reject(new DOMException('Отменено', 'AbortError'));
        return;
      }
      signal?.addEventListener('abort', () => {
        clearTimeout(timer);
        reject(new DOMException('Отменено', 'AbortError'));
      }, { once: true });
    });
    const query = normalized(request.query);
    const matchedThemes = themes.filter((theme) => theme.pattern.test(query));
    const terms = [...new Set(query.match(/[\p{L}\p{N}]+/gu) || [])].filter((term) => term.length > 2);
    const ranked = searchableCatalog
      .filter((movie) => (
        (!request.filters.genres.length || request.filters.genres.some((genre) => movie.genres.includes(genre))) &&
        (request.filters.year_from === null || movie.year >= request.filters.year_from) &&
        (request.filters.year_to === null || movie.year <= request.filters.year_to) &&
        (request.filters.min_rating === 0 || (movie.votes > 0 && movie.rating >= request.filters.min_rating))
      ))
      .map((movie) => {
        let score = normalized(movie.title).includes(query) ? 30 : 0;
        for (const term of terms) {
          if (normalized(movie.title).includes(term)) score += 6;
          if (movie.searchText.includes(term)) score += 1;
        }
        for (const theme of matchedThemes) {
          if (theme.ids?.includes(movie.id)) score += 8;
          if (theme.genres?.some((genre) => movie.genres.includes(genre))) score += 4;
          score += theme.terms.filter((term) => movie.searchText.includes(term)).length;
        }
        return { movie, score };
      })
      .filter((hit) => hit.score > 0)
      .sort((a, b) => b.score - a.score || b.movie.rating - a.movie.rating || a.movie.id - b.movie.id);
    return {
      items: ranked.slice(0, request.limit).map(({ movie }) => ({
        ...movie,
        reason: this.explain(movie, matchedThemes),
      })),
      matchedCount: ranked.length,
      requestedCount: request.limit,
    };
  }

  explain(movie, matchedThemes) {
    const theme = matchedThemes.find((entry) => entry.ids?.includes(movie.id) ||
      entry.genres?.some((genre) => movie.genres.includes(genre)) ||
      entry.terms.some((term) => movie.searchText.includes(term)));
    const genres = movie.genres.slice(0, 2).map((genre) => genreNames[genre] || genre).join(', ').toLowerCase();
    return theme
      ? 'Вариант под настроение «' + theme.label + '». ' + capitalize(genres) + '; подробности сюжета — в описании ниже.'
      : 'В описании или названии есть совпадения с вашим запросом. ' + capitalize(genres) + ' с рейтингом ' + formatRating(movie.rating) + '.';
  }
}

const client = new DemoRecommendationClient();
let pending = null;
let requestVersion = 0;
let lastDrawerFocus = null;
const formatRating = (value) => value.toLocaleString('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const capitalize = (value) => value ? value[0].toUpperCase() + value.slice(1) : value;

function readFilters() {
  const form = new FormData(dom.filters);
  return {
    genres: form.getAll('genres'),
    year_from: form.get('year_from') === '' ? null : Number(form.get('year_from')),
    year_to: form.get('year_to') === '' ? null : Number(form.get('year_to')),
    min_rating: Number(form.get('rating')),
  };
}

function filterLabels(filters, count) {
  const labels = filters.genres.map((genre) => genreNames[genre] || genre);
  if (filters.year_from !== null && filters.year_to !== null) labels.push(filters.year_from + '–' + filters.year_to);
  else if (filters.year_from !== null) labels.push('от ' + filters.year_from + ' года');
  else if (filters.year_to !== null) labels.push('до ' + filters.year_to + ' года');
  if (filters.min_rating) labels.push('рейтинг ' + filters.min_rating + '+');
  if (count !== undefined) labels.push('до ' + count + ' фильмов');
  return labels;
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
  rating.append(icon('star'), document.createTextNode(formatRating(movie.rating)));
  top.append(element('h3', '', movie.title), rating);
  const meta = [
    movie.year,
    movie.genres.map((genre) => genreNames[genre] || genre).join(' / '),
    movie.runtime ? movie.runtime + ' мин' : null,
  ].filter(Boolean).join(' · ');
  const details = element('details', 'movie-details');
  const overview = element('p', 'movie-overview', movie.overview);
  overview.lang = 'en';
  details.append(element('summary', '', 'О фильме · описание на английском'), overview);
  content.append(top, element('p', 'movie-meta', meta), element('p', 'movie-reason', movie.reason), details);
  card.append(element('span', 'movie-number', String(index + 1).padStart(2, '0')), content);
  return card;
}

function renderResponse(response) {
  const message = assistantMessage();
  if (!response.items.length) {
    message.append(element('p', 'assistant-text', 'В демоколлекции ничего не нашлось. Попробуйте другую тему, название на английском или немного смягчите фильтры.'));
  } else {
    message.append(element('p', 'assistant-text', 'Вот что можно посмотреть. Выбрал фильмы по вашему запросу и учёл настройки слева.'));
    const list = element('div', 'movie-list');
    response.items.forEach((movie, index) => list.append(renderMovie(movie, index)));
    message.append(list);
    if (response.items.length < response.requestedCount) {
      message.append(element('p', 'result-note', 'Подходит ' + response.items.length + ' из запрошенных ' + response.requestedCount + '. В этой демоверсии всего ' + catalog.length + ' фильмов — попробуйте расширить условия.'));
    }
    message.append(element('p', 'result-note', 'Можно уточнить запрос или начать новый подбор.'));
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
  if (!query || pending) return;
  if (!validateRequest()) return;
  showError('');
  const request = { query, filters: readFilters(), limit: Number(dom.count.value) };
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
      loading.replaceChildren(element('p', 'assistant-text', 'Не получилось завершить подбор. Попробуйте отправить запрос ещё раз.'));
      showError('Произошла ошибка. Ваш запрос сохранён в поле ввода.');
    }
  } finally {
    if (version === requestVersion) {
      pending = null;
      setBusy(false);
    }
  }
}

function resetConversation() {
  requestVersion += 1;
  pending?.abort();
  pending = null;
  dom.messages.replaceChildren();
  dom.workspace.classList.remove('has-messages');
  dom.conversation.hidden = true;
  dom.welcome.hidden = false;
  dom.suggestions.hidden = false;
  dom.query.value = '';
  dom.query.style.height = '';
  status.textContent = '';
  setBusy(false);
  showError('');
  dom.query.focus({ preventScroll: true });
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
  dom.count.value = String(Math.min(50, (Number(dom.count.value) || 10) + 1));
});
$('#new-chat').addEventListener('click', resetConversation);
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
$('#about-catalog-count').textContent = String(catalog.length);
updateFilters();
setDrawer(false);
