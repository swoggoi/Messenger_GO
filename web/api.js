/* БипЧат API client — адаптер к REST API Messenger_GO. */
(function () {
  'use strict';

  var API_PREFIX = '/api/v1';
  var TOKEN_KEY = 'bipchat_token';
  var USER_KEY = 'bipchat_user';

  function getToken() {
    return localStorage.getItem(TOKEN_KEY) || '';
  }

  function saveAuth(token, user) {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
  }

  function clearAuth() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
  }

  function readErrorBody(text) {
    if (!text) return '';
    try {
      var data = JSON.parse(text);
      return data.error || data.message || text;
    } catch (_) {
      return text;
    }
  }

  async function request(path, options) {
    options = options || {};
    var headers = new Headers(options.headers || {});
    var token = getToken();
    if (token) headers.set('Authorization', 'Bearer ' + token);
    if (options.body !== undefined && !(options.body instanceof FormData) && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json');
    }

    var response;
    try {
      response = await fetch(API_PREFIX + path, Object.assign({}, options, {
        headers: headers,
        credentials: 'same-origin'
      }));
    } catch (error) {
      var networkError = new Error('Не удалось подключиться к серверу. Проверьте, запущен ли БипЧат.');
      networkError.cause = error;
      throw networkError;
    }

    var text = await response.text();
    var data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (_) { data = text; }
    }

    if (!response.ok) {
      if (response.status === 401 && path !== '/auth/login' && path !== '/auth/register') {
        clearAuth();
      }
      var message = typeof data === 'string' ? readErrorBody(data) : (data && (data.error || data.message));
      var error = new Error(message || ('Ошибка сервера (' + response.status + ')'));
      error.status = response.status;
      error.data = data;
      throw error;
    }
    return data;
  }

  var api = {
    getToken: getToken,
    getCachedUser: function () {
      try { return JSON.parse(localStorage.getItem(USER_KEY) || 'null'); }
      catch (_) { return null; }
    },
    saveAuth: saveAuth,
    clearAuth: clearAuth,

    register: function (payload) {
      return request('/auth/register', { method: 'POST', body: JSON.stringify(payload) });
    },
    login: async function (email, password) {
      var response = await request('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email: email, password: password })
      });
      if (!response || !response.token) throw new Error('Сервер не вернул токен авторизации.');
      saveAuth(response.token, response.user);
      return response;
    },
    me: async function () {
      var user = await request('/users/me');
      localStorage.setItem(USER_KEY, JSON.stringify(user));
      return user;
    },
    updateProfile: async function (payload) {
      var response = await request('/users/profile', {
        method: 'PUT',
        body: JSON.stringify(payload)
      });
      var user = response && response.user ? response.user : response;
      if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
      return user;
    },
    searchUsers: function (query) {
      return request('/users/search?q=' + encodeURIComponent(query || ''));
    },
    getUser: function (userID) {
      return request('/users/' + encodeURIComponent(userID));
    },
    getChats: function () {
      return request('/chats/');
    },
    createPrivateChat: function (userID) {
      return request('/chats/private', {
        method: 'POST',
        body: JSON.stringify({ user_id: userID })
      });
    },
    createGroupChat: function (name, userIDs) {
      return request('/chats/group', {
        method: 'POST',
        body: JSON.stringify({ name: name, user_ids: userIDs })
      });
    },
    getMessages: function (chatID, limit, offset) {
      var params = new URLSearchParams({ limit: String(limit || 50), offset: String(offset || 0) });
      return request('/chats/' + encodeURIComponent(chatID) + '/messages?' + params.toString());
    },
    sendMessage: function (chatID, content, replyToID) {
      var body = { content: content };
      if (replyToID) body.reply_to_id = replyToID;
      return request('/chats/' + encodeURIComponent(chatID) + '/messages', {
        method: 'POST',
        body: JSON.stringify(body)
      });
    },
    deleteMessage: function (chatID, messageID) {
      return request('/chats/' + encodeURIComponent(chatID) + '/messages/' + encodeURIComponent(messageID), {
        method: 'DELETE'
      });
    },
    togglePin: function (chatID, messageID) {
      return request('/chats/' + encodeURIComponent(chatID) + '/messages/' + encodeURIComponent(messageID) + '/pin', {
        method: 'POST'
      });
    },
    markAsRead: function (chatID) {
      return request('/chats/' + encodeURIComponent(chatID) + '/read', { method: 'POST' });
    },
    logout: function () {
      // Текущий Go backend использует stateless JWT и не имеет endpoint отзыва токена.
      clearAuth();
      return Promise.resolve();
    }
  };

  window.BipAPI = api;
})();
