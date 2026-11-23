export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function request(path, { method = 'GET', body, signal, token } = {}) {
  const headers = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = 'Bearer ' + token;
  const response = await fetch('/api/v1' + path, {
    method,
    headers,
    signal,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return null;
  const payload = await response.json().catch((error) => {
    if (error.name === 'AbortError') throw error;
    return null;
  });
  if (!response.ok || !payload) {
    throw new ApiError(response.status, payload?.error?.message || 'Сервер не вернул ожидаемый ответ.');
  }
  return payload;
}

// Refresh rotates a shared browser cookie. Serialize refreshes across tabs when
// Web Locks is available, and reuse the same request within a tab.
function withRefreshLock(action) {
  const locks = globalThis.navigator?.locks;
  return locks ? locks.request('movie-picker-auth', action) : action();
}

export class MoviePickerApi {
  #token = '';
  #userId = null;
  #session = 0;
  #refreshing = null;

  async login(body) {
    return withRefreshLock(async () => {
      const session = await request('/auth/login', { method: 'POST', body });
      this.#session += 1;
      this.#token = session.access_token;
      this.#userId = session.user.id;
      return session.user;
    });
  }

  async restore() {
    try {
      await this.#refresh();
      const user = await this.#authorized('/auth/me');
      this.#userId = user.id;
      return user;
    } catch (error) {
      if (error.status === 401) {
        this.clearSession();
        return null;
      }
      throw error;
    }
  }

  async logout() {
    await withRefreshLock(async () => {
      await request('/auth/logout', { method: 'POST' });
      this.clearSession();
    });
  }

  clearSession() {
    this.#session += 1;
    this.#token = '';
    this.#userId = null;
    this.#refreshing = null;
  }

  catalog({ signal, guest = false } = {}) {
    if (guest) return request('/guest/catalog', { signal });
    return this.#authorized('/catalog', { signal });
  }

  recommend(body, { signal, guest = false } = {}) {
    if (guest) return request('/guest/recommendations', { method: 'POST', body, signal });
    return this.#authorized('/recommendations', { method: 'POST', body, signal });
  }

  listChats({ signal } = {}) {
    return this.#authorized('/chats', { signal });
  }

  getChat(id, { signal } = {}) {
    return this.#authorized('/chats/' + encodeURIComponent(id), { signal });
  }

  listMovies(status, page, pageSize, { signal } = {}) {
    const query = new URLSearchParams({ status, page, page_size: pageSize });
    return this.#authorized('/my/movies?' + query, { signal });
  }

  saveMovie(id, status, liked, { signal } = {}) {
    return this.#authorized('/my/movies/' + id, { method: 'PUT', body: { status, liked }, signal });
  }

  removeMovie(id, { signal } = {}) {
    return this.#authorized('/my/movies/' + id, { method: 'DELETE', signal });
  }

  #refresh() {
    if (!this.#refreshing) {
      const version = this.#session;
      const refreshing = withRefreshLock(async () => {
        this.#assertSession(version);
        try {
          const session = await request('/auth/refresh', { method: 'POST' });
          this.#assertSession(version);
          if (this.#userId != null) {
            const user = await request('/auth/me', { token: session.access_token });
            this.#assertSession(version);
            if (user.id !== this.#userId) throw new ApiError(401, 'Аккаунт изменился. Войдите снова.');
          }
          this.#token = session.access_token;
        } catch (error) {
          if (error.status === 401 && version === this.#session) this.#token = '';
          throw error;
        }
      }).finally(() => {
        if (this.#refreshing === refreshing) this.#refreshing = null;
      });
      this.#refreshing = refreshing;
    }
    return this.#refreshing;
  }

  #assertSession(version) {
    if (version !== this.#session) throw new DOMException('Session changed', 'AbortError');
  }

  async #authorized(path, options = {}) {
    const token = this.#token;
    const version = this.#session;
    if (!token) throw new ApiError(401, 'Войдите, чтобы продолжить.');
    try {
      return await request(path, { ...options, token });
    } catch (error) {
      if (error.status !== 401) throw error;
      this.#assertSession(version);
      options.signal?.throwIfAborted();
      if (token === this.#token) await this.#refresh();
      this.#assertSession(version);
      options.signal?.throwIfAborted();
      return request(path, { ...options, token: this.#token });
    }
  }
}
