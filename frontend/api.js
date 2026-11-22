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
  #refreshing = null;

  async authenticate(mode, body) {
    return withRefreshLock(async () => {
      const session = await request('/auth/' + mode, { method: 'POST', body });
      this.#token = session.access_token;
      return session.user;
    });
  }

  async restore() {
    try {
      await this.#refresh();
      return await this.#authorized('/auth/me');
    } catch (error) {
      if (error.status === 401) return null;
      throw error;
    }
  }

  async logout() {
    await withRefreshLock(async () => {
      await request('/auth/logout', { method: 'POST' });
      this.#token = '';
    });
  }

  catalog() {
    return this.#authorized('/catalog');
  }

  recommend(body, { signal } = {}) {
    return this.#authorized('/recommendations', { method: 'POST', body, signal });
  }

  #refresh() {
    if (!this.#refreshing) {
      this.#refreshing = withRefreshLock(async () => {
        try {
          const session = await request('/auth/refresh', { method: 'POST' });
          this.#token = session.access_token;
        } catch (error) {
          if (error.status === 401) this.#token = '';
          throw error;
        }
      }).finally(() => { this.#refreshing = null; });
    }
    return this.#refreshing;
  }

  async #authorized(path, options = {}) {
    const token = this.#token;
    if (!token) throw new ApiError(401, 'Войдите, чтобы продолжить.');
    try {
      return await request(path, { ...options, token });
    } catch (error) {
      if (error.status !== 401) throw error;
      options.signal?.throwIfAborted();
      if (token === this.#token) await this.#refresh();
      return request(path, { ...options, token: this.#token });
    }
  }
}
