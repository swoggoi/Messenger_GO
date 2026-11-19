/* БипЧат — интерфейс и подключение страниц к REST API Messenger_GO. */
(function () {
  'use strict';

  var api = window.BipAPI;
  var currentUser = null;
  var currentChat = null;
  var currentMessages = [];
  var messagePoll = null;
  var wsSocket = null;
  var wsReconnect = null;
  var chatListRefresh = null;
  var audioContext = null;
  var userAvatarCache = Object.create(null);
  var replyTarget = null;
  var selectionMode = false;
  var selectedMessages = Object.create(null);
  var contextMenu = null;

  var SETTINGS_KEY = 'bipchat:settings';
  var CONTACTS_KEY = 'bipchat:contacts';
  var LOCAL_PROFILE_KEY = 'bipchat:local-profile';

  function readContacts() {
    try {
      var value = JSON.parse(localStorage.getItem(CONTACTS_KEY) || '[]');
      return Array.isArray(value) ? value : [];
    } catch (_) {
      return [];
    }
  }

  function saveContacts(contacts) {
    localStorage.setItem(CONTACTS_KEY, JSON.stringify(contacts));
  }

  function isContact(userID) {
    return readContacts().some(function (contact) { return contact.id === userID; });
  }

  function contactByID(userID) {
    if (!userID) return null;
    return readContacts().find(function (contact) { return contact.id === userID; }) || null;
  }

  // Псевдоним контакта живёт только в этом браузере.
  function contactLabel(user) {
    if (!user) return 'Пользователь';
    var contact = contactByID(user.id);
    return (contact && contact.alias) || user.name || user.username || 'Пользователь';
  }

  function senderLabel(entry) {
    if (!entry) return 'Участник';
    if (entry.sender_id === (currentUser && currentUser.id)) return entry.sender_name || 'Вы';
    return contactLabel({ id: entry.sender_id, name: entry.sender_name });
  }

  function readLocalProfile() {
    try {
      return JSON.parse(localStorage.getItem(LOCAL_PROFILE_KEY) || '{}') || {};
    } catch (_) {
      return {};
    }
  }

  function writeContact(userID, patch) {
    var contacts = readContacts();
    var index = contacts.findIndex(function (contact) { return contact.id === userID; });
    if (index >= 0) contacts[index] = Object.assign({}, contacts[index], patch);
    else contacts.push(Object.assign({ id: userID }, patch));
    saveContacts(contacts);
    renderSavedContacts();
    scheduleChatListRefresh();
  }

  function toggleContact(user) {
    if (!user || !user.id) return false;
    var contacts = readContacts();
    var index = contacts.findIndex(function (contact) { return contact.id === user.id; });
    if (index >= 0) {
      var removed = contacts.splice(index, 1)[0];
      saveContacts(contacts);
      renderSavedContacts();
      applyContactAliasToUI(user.id, removed.name || removed.username || '');
      showToast('Удалено из контактов.');
      return false;
    }
    contacts.push({
      id: user.id,
      name: user.name || user.username || 'Пользователь',
      username: user.username || '',
      avatar: user.avatar_url || '',
      email: user.email || '',
      alias: '',
      showEmail: false
    });
    saveContacts(contacts);
    renderSavedContacts();
    showToast('Добавлено в контакты.');
    return true;
  }

  function renderSavedContacts() {
    var list = document.getElementById('saved-contacts');
    if (!list) return;
    var count = document.getElementById('saved-count');
    var contacts = readContacts();
    if (count) count.textContent = contacts.length ? '(' + contacts.length + ')' : '';
    if (!contacts.length) {
      list.innerHTML = '<div class="list-state">Пока нет контактов. Откройте профиль пользователя и нажмите «Добавить в контакты».</div>';
      return;
    }
    list.innerHTML = contacts.map(function (contact) {
      var label = contact.alias || contact.name || withAt(contact.username) || 'Пользователь';
      var showMail = !!contact.showEmail || isSettingEnabled(readSettings(), 'showEmail');
      var email = showMail ? (contact.email || '') : '';
      var at = withAt(contact.username);
      var desc = at && email ? at + ' · ' + email : (at || email);
      rememberAvatar(contact.id, contact.avatar);
      return '<div class="row contact center contact-result" data-user-id="' + escapeHTML(contact.id) + '">' +
        '<button class="contact-open" type="button" data-profile-id="' + escapeHTML(contact.id) + '" title="Открыть профиль">' +
          avatarSpan(contact.id, label, 'avatar') +
          '<span class="meta"><span class="name">' + escapeHTML(label) + '</span><span class="desc">' + escapeHTML(desc) + '</span></span>' +
        '</button>' +
        '<button class="btn btn-mini" type="button" data-edit-contact="' + escapeHTML(contact.id) + '" title="Изменить имя и почту только у себя">Изменить</button>' +
        '<button class="btn btn-mini" type="button" data-remove-contact="' + escapeHTML(contact.id) + '" title="Удалить из контактов">Удалить</button></div>';
    }).join('');
  }

  function openContactEditor(userID, baseUser) {
    if (!userID) return;
    var modal = document.getElementById('contact-edit-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'contact-edit-modal';
      modal.className = 'modal-overlay';
      modal.setAttribute('role', 'dialog');
      modal.setAttribute('aria-modal', 'true');
      modal.innerHTML = '<div class="modal-card local-card">' +
        '<div class="modal-head"><h2>Данные контакта только у вас</h2>' +
        '<button class="icon-btn" type="button" id="ce-close" aria-label="Закрыть"><svg viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12"/></svg></button></div>' +
        '<div class="local-body">' +
        '<p class="about-text">Эти значения хранятся только в вашем браузере. Другой человек их не увидит.</p>' +
        '<div class="field"><label for="ce-name">Ник (только у вас)</label><input class="input" id="ce-name" maxlength="120" placeholder="Как отображать в чате"></div>' +
        '<div class="field"><label for="ce-email">Почта (только у вас)</label><input class="input" id="ce-email" type="email" maxlength="200" placeholder="Заметка"></div>' +
        '<div class="switch-row ce-switch"><div><div class="t">Показывать почту</div><div class="d">Отображать эту почту в контактах и профиле</div></div>' +
        '<button class="toggle" type="button" role="switch" id="ce-show-email" aria-label="Показывать почту контакта"></button></div>' +
        '</div>' +
        '<div class="modal-foot contact-edit-foot">' +
        '<button class="btn" type="button" id="ce-reset">Сбросить</button>' +
        '<button class="btn" type="button" id="ce-cancel">Отмена</button>' +
        '<button class="btn primary" type="button" id="ce-save">Сохранить</button></div></div>';
      document.body.appendChild(modal);
      modal.addEventListener('click', function (event) { if (event.target === modal) modal.classList.remove('open'); });
      document.getElementById('ce-close').addEventListener('click', function () { modal.classList.remove('open'); });
      document.getElementById('ce-cancel').addEventListener('click', function () { modal.classList.remove('open'); });
      document.getElementById('ce-show-email').addEventListener('click', function () {
        var on = this.getAttribute('aria-checked') !== 'true';
        this.classList.toggle('on', on);
        this.setAttribute('aria-checked', on ? 'true' : 'false');
      });
      document.addEventListener('keydown', function (event) {
        if (event.key === 'Escape' && modal.classList.contains('open')) modal.classList.remove('open');
      });

      function refreshProfile(id) {
        var profile = document.getElementById('profile-modal');
        if (profile && profile.classList.contains('open')) openUserProfile(id);
      }

      document.getElementById('ce-save').addEventListener('click', function () {
        var id = modal.getAttribute('data-user-id');
        if (!id) return;
        var base = modal._base || {};
        var existing = contactByID(id) || {};
        var alias = document.getElementById('ce-name').value.trim();
        writeContact(id, {
          name: existing.name || base.name || base.username || contactLabel(base),
          username: existing.username || base.username || '',
          avatar: existing.avatar || base.avatar || base.avatar_url || '',
          email: document.getElementById('ce-email').value.trim(),
          alias: alias,
          showEmail: document.getElementById('ce-show-email').getAttribute('aria-checked') === 'true'
        });
        applyContactAliasToUI(id, alias || existing.name || base.name || '');
        modal.classList.remove('open');
        showToast('Сохранено только у вас.');
        refreshProfile(id);
      });

      document.getElementById('ce-reset').addEventListener('click', function () {
        var id = modal.getAttribute('data-user-id');
        if (!id) return;
        var base = modal._base || {};
        writeContact(id, {
          email: base.email || '',
          alias: '',
          showEmail: false
        });
        applyContactAliasToUI(id, base.name || base.username || '');
        modal.classList.remove('open');
        showToast('Локальные изменения сброшены.');
        refreshProfile(id);
      });
    }

    var contact = contactByID(userID);
    var cached = null;
    try {
      cached = JSON.parse(localStorage.getItem('bipchat:user-profile:' + userID) || 'null');
    } catch (_) {
      // Игнорируем повреждённый кэш.
    }
    var base = contact || baseUser || cached || {};
    modal._base = base;
    modal.setAttribute('data-user-id', userID);
    document.getElementById('ce-name').value = (contact && contact.alias) || '';
    document.getElementById('ce-email').value = (contact && contact.email) || (base.email || '');
    var show = contact ? !!contact.showEmail : isSettingEnabled(readSettings(), 'showEmail');
    var toggle = document.getElementById('ce-show-email');
    toggle.classList.toggle('on', show);
    toggle.setAttribute('aria-checked', show ? 'true' : 'false');
    modal.classList.add('open');
    document.getElementById('ce-name').focus();
  }

  function withoutAt(value) {
    return String(value == null ? '' : value).trim().replace(/^@+/, '');
  }

  function withAt(value) {
    var username = withoutAt(value);
    return username ? '@' + username : '';
  }

  function readSettings() {
    try {
      return JSON.parse(localStorage.getItem(SETTINGS_KEY) || '{}') || {};
    } catch (_) {
      return {};
    }
  }

  function isSettingEnabled(settings, key) {
    if (Object.prototype.hasOwnProperty.call(settings, key)) return !!settings[key];
    // Превью включены по умолчанию, остальные переключатели — выключены.
    return key === 'preview';
  }

  function saveSettings(next) {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(next));
    applySettings(next);
  }

  function applySettings(settings) {
    document.body.classList.toggle('compact', !!settings.compact);
    document.body.classList.toggle('large-text', !!settings.largeText);
  }

  function playNotificationSound() {
    try {
      var Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return;
      if (!audioContext) audioContext = new Ctx();
      if (audioContext.state === 'suspended') {
        var resumed = audioContext.resume();
        if (resumed && typeof resumed.catch === 'function') resumed.catch(function () {});
      }
      var oscillator = audioContext.createOscillator();
      var gain = audioContext.createGain();
      oscillator.type = 'sine';
      oscillator.frequency.value = 660;
      gain.gain.setValueAtTime(0.0001, audioContext.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.12, audioContext.currentTime + 0.015);
      gain.gain.exponentialRampToValueAtTime(0.0001, audioContext.currentTime + 0.2);
      oscillator.connect(gain);
      gain.connect(audioContext.destination);
      oscillator.start();
      oscillator.stop(audioContext.currentTime + 0.21);
    } catch (_) {
      // Браузер может запретить воспроизведение до первого действия пользователя.
    }
  }

  function showDesktopNotification(payload) {
    if (!('Notification' in window) || Notification.permission !== 'granted') return;
    var title = payload.sender_name || 'Новое сообщение';
    try {
      new Notification(title, { body: payload.content || '', tag: payload.chat_id || '' });
    } catch (_) {
      // Некоторые браузеры запрещают Notification из не-secure контекста.
    }
  }

  function showMessagePreview(payload) {
    var preview = document.getElementById('message-preview');
    if (!preview) {
      preview = document.createElement('button');
      preview.id = 'message-preview';
      preview.type = 'button';
      preview.className = 'message-preview';
      preview.innerHTML = '<span class="pv-avatar"></span><span class="pv-body"><span class="pv-title"></span><span class="pv-text"></span></span>';
      document.body.appendChild(preview);
      preview.addEventListener('click', function () {
        var id = preview.getAttribute('data-chat-id');
        var chats = window.__bipChats || [];
        var chat = chats.find(function (entry) { return entry.id === id; });
        if (chat) openChat(chat, document.querySelector('[data-chat-id="' + CSS.escape(id) + '"]'));
        preview.classList.remove('show');
      });
    }
    var sender = senderLabel(payload);
    preview.querySelector('.pv-avatar').className = 'avatar pv-avatar ' + avatarColor(payload.sender_id || sender);
    preview.querySelector('.pv-avatar').innerHTML = avatarInner(payload.sender_id, sender);
    preview.querySelector('.pv-title').textContent = sender;
    preview.querySelector('.pv-text').textContent = payload.content || '';
    preview.setAttribute('data-chat-id', payload.chat_id || '');
    preview.classList.add('show');
    window.clearTimeout(preview._timer);
    preview._timer = window.setTimeout(function () { preview.classList.remove('show'); }, 5200);
  }

  function notifyIncomingMessage(payload) {
    if (!payload || payload.sender_id === (currentUser && currentUser.id)) return;
    if (currentChat && payload.chat_id === currentChat.id && !document.hidden) return;
    var settings = readSettings();
    if (isSettingEnabled(settings, 'sound')) playNotificationSound();
    if (isSettingEnabled(settings, 'desktop')) showDesktopNotification(payload);
    if (isSettingEnabled(settings, 'preview')) showMessagePreview(payload);
  }

  function closeUserProfile() {
    var modal = document.getElementById('profile-modal');
    if (modal) modal.classList.remove('open');
  }

  async function openUserProfile(userID) {
    if (!userID) return;
    var modal = document.getElementById('profile-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'profile-modal';
      modal.className = 'modal-overlay';
      modal.setAttribute('role', 'dialog');
      modal.setAttribute('aria-modal', 'true');
      modal.setAttribute('aria-labelledby', 'profile-title');
      modal.innerHTML = '<div class="modal-card profile-card">' +
        '<div class="modal-head"><h2 id="profile-title">Профиль пользователя</h2>' +
        '<button class="icon-btn" type="button" id="profile-close" aria-label="Закрыть"><svg viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12"/></svg></button></div>' +
        '<div class="profile-modal-body" id="profile-modal-body"></div>' +
        '<div class="modal-foot profile-foot">' +
          '<button class="btn" type="button" id="profile-edit">Изменить</button>' +
          '<button class="btn" type="button" id="profile-contact">Добавить в контакты</button>' +
          '<button class="btn primary" type="button" id="profile-write">Написать</button>' +
        '</div>' +
        '</div>';
      document.body.appendChild(modal);
      modal.addEventListener('click', function (event) { if (event.target === modal) closeUserProfile(); });
      document.getElementById('profile-close').addEventListener('click', closeUserProfile);
      document.addEventListener('keydown', function (event) { if (event.key === 'Escape') closeUserProfile(); });
      document.getElementById('profile-write').addEventListener('click', async function () {
        var id = modal.getAttribute('data-user-id');
        if (!id) return;
        var button = this;
        setBusy(button, true, 'Открываем…');
        try {
          var chat = await api.createPrivateChat(id);
          localStorage.setItem('bipchat:chat-name:' + chat.id, modal.getAttribute('data-user-name') || '');
          rememberAvatar(id, modal._profileUser && modal._profileUser.avatar_url || '');
          localStorage.setItem('bipchat:selected-chat', chat.id);
          closeUserProfile();
          window.location.href = 'messages.html';
        } catch (error) {
          showToast(friendlyError(error));
          setBusy(button, false);
        }
      });
      document.getElementById('profile-contact').addEventListener('click', function () {
        var added = toggleContact(modal._profileUser);
        this.textContent = added ? 'В контактах' : 'Добавить в контакты';
        this.classList.toggle('primary', added);
        document.getElementById('profile-edit').hidden = !added;
      });
      document.getElementById('profile-edit').addEventListener('click', function () {
        var id = modal.getAttribute('data-user-id');
        if (id) openContactEditor(id, modal._profileUser);
      });
    }

    modal.setAttribute('data-user-id', userID);
    var isSelf = userID === (currentUser && currentUser.id);
    var write = document.getElementById('profile-write');
    var contact = document.getElementById('profile-contact');
    var edit = document.getElementById('profile-edit');
    if (write) write.hidden = isSelf;
    if (contact) contact.hidden = isSelf;
    if (edit) edit.hidden = isSelf || !isContact(userID);
    modal._profileUser = null;
    modal.classList.add('open');
    var body = document.getElementById('profile-modal-body');
    body.innerHTML = '<div class="list-state">Загружаем профиль…</div>';

    try {
      var user = await api.getUser(userID);
      if (modal.getAttribute('data-user-id') !== userID) return;
      modal._profileUser = user;
      rememberAvatar(userID, user.avatar_url);
      if (contact) {
        var added = isContact(userID);
        contact.textContent = added ? 'В контактах' : 'Добавить в контакты';
        contact.classList.toggle('primary', added);
      }
      if (edit) edit.hidden = !isContact(userID);
      var local = readLocalProfile();
      var storedContact = contactByID(userID);
      var fullName = isSelf
        ? (local.name || user.name || user.username || 'Пользователь')
        : ((storedContact && storedContact.alias) || user.name || user.username || 'Пользователь');
      var emailValue;
      if (isSelf) emailValue = local.email || user.email || '—';
      else if (storedContact && storedContact.showEmail) emailValue = storedContact.email || user.email || '—';
      else if (isSettingEnabled(readSettings(), 'showEmail')) emailValue = (storedContact && storedContact.email) || user.email || '—';
      else emailValue = 'Скрыто пользователем';
      modal.setAttribute('data-user-name', fullName);
      var avatarMarkup = user.avatar_url
        ? '<img class="profile-avatar" src="' + escapeHTML(user.avatar_url) + '" alt="">'
        : '<span class="avatar profile-avatar ' + avatarColor(user.id || userID) + '">' + escapeHTML(initials(fullName)) + '</span>';
      body.innerHTML = '<div class="profile-modal-top">' + avatarMarkup +
        '<div class="profile-modal-identity"><div class="profile-modal-name">' + escapeHTML(fullName) + '</div>' +
        '<div class="profile-modal-username">' + escapeHTML(withAt(user.username)) + '</div></div></div>' +
        '<div class="profile-facts">' +
          '<div class="profile-fact"><span>Имя и фамилия</span><strong>' + escapeHTML(fullName) + '</strong></div>' +
          '<div class="profile-fact"><span>Username</span><strong>' + escapeHTML(withAt(user.username)) + '</strong></div>' +
          '<div class="profile-fact"><span>Учебная группа</span><strong>' + escapeHTML(user.group_name || 'Не указана') + '</strong></div>' +
          '<div class="profile-fact"><span>Почта</span><strong>' + escapeHTML(emailValue) + '</strong></div>' +
        '</div>';
    } catch (error) {
      body.innerHTML = '<div class="list-state error-state">' + escapeHTML(friendlyError(error)) + '</div>';
    }
  }

  function initAgreementModal() {
    var modal = document.getElementById('agreement-modal');
    if (!modal) return;

    function open(event) {
      if (event) {
        event.preventDefault();
        event.stopPropagation();
      }
      modal.classList.add('open');
      var closeButton = document.getElementById('agreement-close');
      if (closeButton) closeButton.focus();
    }
    function close() { modal.classList.remove('open'); }

    document.querySelectorAll('[data-agreement]').forEach(function (link) {
      link.addEventListener('click', open);
    });
    var closeButton = document.getElementById('agreement-close');
    if (closeButton) closeButton.addEventListener('click', close);
    var accept = document.getElementById('agreement-accept');
    if (accept) {
      accept.addEventListener('click', function () {
        var form = document.getElementById('reg-form');
        if (form && form.elements.terms) form.elements.terms.checked = true;
        close();
      });
    }
    modal.addEventListener('click', function (event) { if (event.target === modal) close(); });
    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && modal.classList.contains('open')) close();
    });
  }

  var ICONS = {
    chat: '<svg viewBox="0 0 24 24"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>',
    user: '<svg viewBox="0 0 24 24"><circle cx="12" cy="8" r="4"/><path d="M4 21v-1a6 6 0 0 1 6-6h4a6 6 0 0 1 6 6v1"/></svg>',
    users: '<svg viewBox="0 0 24 24"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
    folder: '<svg viewBox="0 0 24 24"><path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z"/></svg>',
    sliders: '<svg viewBox="0 0 24 24"><path d="M21 4h-7M10 4H3M21 12h-9M8 12H3M21 20h-5M12 20H3M14 2v4M8 10v4M16 18v4"/></svg>'
  };

  var NAV = [
    ['messages.html', 'chat', 'Сообщения'],
    ['contacts.html', 'user', 'Контакты'],
    ['groups.html', 'users', 'Группы'],
    ['files.html', 'folder', 'Файлы'],
    ['settings.html', 'sliders', 'Настройки']
  ];

  window.showToast = function (message) {
    var toast = document.getElementById('toast');
    if (!toast) {
      toast = document.createElement('div');
      toast.id = 'toast';
      toast.className = 'toast';
      document.body.appendChild(toast);
    }
    toast.textContent = String(message || 'Готово');
    toast.classList.add('show');
    window.clearTimeout(toast._timer);
    toast._timer = window.setTimeout(function () { toast.classList.remove('show'); }, 2600);
  };

  function escapeHTML(value) {
    return String(value == null ? '' : value).replace(/[&<>"']/g, function (char) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char];
    });
  }

  function initials(name) {
    var parts = String(name || '?').trim().split(/\s+/).filter(Boolean);
    return ((parts[0] || '?').charAt(0) + (parts[1] || '').charAt(0)).toLocaleUpperCase('ru-RU');
  }

  function formatTime(value) {
    if (!value) return '';
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' }).format(date);
  }

  function formatDay(value) {
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'Сегодня';
    var today = new Date();
    var yesterday = new Date();
    yesterday.setDate(today.getDate() - 1);
    if (date.toDateString() === today.toDateString()) return 'Сегодня';
    if (date.toDateString() === yesterday.toDateString()) return 'Вчера';
    return new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' }).format(date);
  }

  function avatarColor(seed) {
    var palette = ['c-slate', 'c-olive', 'c-moss', 'c-me'];
    var value = 0;
    String(seed || '').split('').forEach(function (char) { value = (value + char.charCodeAt(0)) % 997; });
    return palette[value % palette.length];
  }

  function rememberAvatar(userID, url) {
    if (userID) userAvatarCache[userID] = url || '';
  }

  function avatarInner(userID, name) {
    var url = userAvatarCache[userID] || '';
    if (url) return '<img class="avatar-img" src="' + escapeHTML(url) + '" alt="">';
    return escapeHTML(initials(name));
  }

  function avatarSpan(userID, name, className) {
    return '<span class="' + className + ' ' + avatarColor(userID || name) + '">' + avatarInner(userID, name) + '</span>';
  }

  // Бэкенд называет личные чаты "private"; старый фронтенд использовал "direct".
  function isPrivateChat(chat) {
    return !!chat && (chat.type === 'private' || chat.type === 'direct');
  }

  function renderNavigation() {
    var rail = document.getElementById('rail');
    if (!rail) return;
    var active = rail.getAttribute('data-active');
    var local = readLocalProfile();
    rail.innerHTML = '<a class="logo" href="messages.html" title="БипЧат">Б</a>' + NAV.map(function (item) {
      return '<a class="rail-link' + (item[0] === active ? ' active' : '') + '" href="' + item[0] + '" title="' + item[2] + '" aria-label="' + item[2] + '">' + ICONS[item[1]] + '</a>';
    }).join('') + '<div class="rail-bottom"><div class="avatar sm c-me" id="nav-avatar" title="Мой профиль" style="cursor:pointer">' +
      escapeHTML(initials(local.name || (currentUser && currentUser.name))) + '</div></div>';

    var navAvatar = document.getElementById('nav-avatar');
    if (navAvatar && currentUser) {
      navAvatar.addEventListener('click', function () { openUserProfile(currentUser.id); });
    }

    var appRoot = document.querySelector('.app');
    if (appRoot && !document.querySelector('.mobile-nav')) {
      var mobile = document.createElement('nav');
      mobile.className = 'mobile-nav';
      mobile.setAttribute('aria-label', 'Основная навигация');
      mobile.innerHTML = NAV.map(function (item) {
        var label = item[2] === 'Сообщения' ? 'Чаты' : item[2];
        return '<a class="mobile-nav-link' + (item[0] === active ? ' active' : '') + '" href="' + item[0] + '">' + ICONS[item[1]] + '<span>' + label + '</span></a>';
      }).join('');
      appRoot.appendChild(mobile);
    }
  }

  function showError(container, message) {
    if (!container) return;
    container.textContent = message || 'Не удалось выполнить запрос.';
    container.classList.add('visible');
  }

  async function requireAuth() {
    if (!api.getToken()) {
      window.location.replace('login.html');
      return false;
    }
    try {
      currentUser = await api.me();
      return true;
    } catch (error) {
      api.clearAuth();
      window.location.replace('login.html');
      return false;
    }
  }

  async function initAuthPage() {
    initAgreementModal();
    if (api.getToken()) {
      try {
        await api.me();
        window.location.replace('messages.html');
        return;
      } catch (_) {
        api.clearAuth();
      }
    }

    var tabs = document.getElementById('auth-tabs');
    if (tabs) {
      tabs.addEventListener('click', function (event) {
        var button = event.target.closest('[data-view]');
        if (!button) return;
        var view = button.getAttribute('data-view');
        tabs.querySelectorAll('[data-view]').forEach(function (item) { item.classList.toggle('active', item === button); });
        document.querySelectorAll('.view').forEach(function (item) { item.classList.toggle('active', item.id === 'view-' + view); });
      });
    }

    var registerForm = document.getElementById('reg-form');
    if (registerForm) {
      registerForm.addEventListener('submit', async function (event) {
        event.preventDefault();
        var errorNode = document.getElementById('reg-error');
        errorNode.textContent = '';
        var fields = registerForm.elements;
        var firstName = fields.fname.value.trim();
        var lastName = fields.lname.value.trim();
        var username = withoutAt(fields.username.value);
        var email = fields.email.value.trim().toLowerCase();
        var password = fields.password.value;
        var group = fields.group ? fields.group.value : '';
        var validation = '';

        if (!firstName || !lastName) validation = 'Укажите имя и фамилию.';
        else if (!/^[a-z0-9_]{3,32}$/i.test(username)) validation = 'Username: 3–32 символа, латиница, цифры и нижнее подчёркивание.';
        else if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) validation = 'Введите корректный адрес электронной почты.';
        else if (!['Т-491', 'Т-492', 'Т-493', 'Т-494', 'Т-495'].includes(group)) validation = 'Выберите учебную группу из списка.';
        else if (password.length < 8 || !/\d/.test(password)) validation = 'Пароль должен содержать минимум 8 символов и хотя бы одну цифру.';
        else if (password !== fields.password2.value) validation = 'Пароли не совпадают.';
        else if (!fields.terms.checked) validation = 'Необходимо принять правила использования.';

        if (validation) {
          showError(errorNode, validation);
          return;
        }

        var submit = registerForm.querySelector('button[type="submit"]');
        setBusy(submit, true, 'Создаём аккаунт…');
        try {
          await api.register({
            username: username,
            name: firstName + ' ' + lastName,
            email: email,
            password: password,
            group_name: group
          });
          // Регистрация текущего backend возвращает User, поэтому сразу выполняем login для получения JWT.
          await api.login(email, password);
          window.location.replace('messages.html');
        } catch (error) {
          showError(errorNode, friendlyError(error));
          setBusy(submit, false);
        }
      });
    }

    var loginForm = document.getElementById('login-form');
    if (loginForm) {
      loginForm.addEventListener('submit', async function (event) {
        event.preventDefault();
        var errorNode = document.getElementById('login-error');
        errorNode.textContent = '';
        var identifier = loginForm.elements.login.value.trim();
        // @username и username — один и тот же логин.
        if (identifier.charAt(0) === '@') identifier = withoutAt(identifier);
        var password = loginForm.elements.lpass.value;
        if (!identifier || !password) {
          showError(errorNode, 'Введите почту или username и пароль.');
          return;
        }
        var submit = loginForm.querySelector('button[type="submit"]');
        setBusy(submit, true, 'Входим…');
        try {
          await api.login(identifier.toLowerCase(), password);
          window.location.replace('messages.html');
        } catch (error) {
          showError(errorNode, friendlyError(error));
          setBusy(submit, false);
        }
      });
    }
  }

  function friendlyError(error) {
    var message = error && error.message ? error.message : 'Не удалось выполнить запрос.';
    if (/user already exists|duplicate|unique/i.test(message)) return 'Пользователь с таким username или почтой уже существует.';
    if (/invalid credentials/i.test(message)) return 'Неверная почта или пароль.';
    if (/all fields are required/i.test(message)) return 'Заполните все обязательные поля.';
    return message;
  }

  function setBusy(button, busy, label) {
    if (!button) return;
    if (busy) {
      button.dataset.originalText = button.textContent;
      button.textContent = label || 'Загрузка…';
      button.disabled = true;
      button.classList.add('is-loading');
    } else {
      button.textContent = button.dataset.originalText || button.textContent;
      button.disabled = false;
      button.classList.remove('is-loading');
    }
  }

  function initSettings() {
    initAgreementModal();

    var preferences = readSettings();
    document.querySelectorAll('.toggle[data-setting]').forEach(function (button) {
      var key = button.getAttribute('data-setting');
      var enabled = isSettingEnabled(preferences, key);
      function render() {
        button.classList.toggle('on', enabled);
        button.setAttribute('aria-checked', enabled ? 'true' : 'false');
      }
      render();
      button.addEventListener('click', function () {
        var current = readSettings();
        enabled = !isSettingEnabled(current, key);
        current[key] = enabled;
        saveSettings(current);
        render();
        if (key === 'desktop' && enabled && 'Notification' in window && Notification.permission === 'default') {
          var requested = Notification.requestPermission();
          if (requested && typeof requested.catch === 'function') requested.catch(function () {});
        }
        showToast(enabled ? 'Включено.' : 'Выключено.');
      });
    });

    var nav = document.querySelector('.settings-nav');
    if (nav) {
      nav.addEventListener('click', function (event) {
        var button = event.target.closest('button[data-tab]');
        if (!button) return;
        nav.querySelectorAll('button').forEach(function (item) { item.classList.toggle('active', item === button); });
        document.querySelectorAll('.tab-panel').forEach(function (panel) {
          panel.classList.toggle('active', panel.id === 'tab-' + button.getAttribute('data-tab'));
        });
      });
    }

    var nameInput = document.getElementById('p-name');
    var userInput = document.getElementById('p-user');
    var emailInput = document.getElementById('p-email');
    if (currentUser) {
      if (nameInput) nameInput.value = currentUser.name || '';
      if (userInput) userInput.value = withAt(currentUser.username);
      if (emailInput) emailInput.value = currentUser.email || '';
      var groupInput = document.getElementById('p-group');
      if (groupInput) groupInput.value = currentUser.group_name || 'Не указана';
      var avatar = document.querySelector('.avatar.xl');
      if (avatar) avatar.textContent = initials(currentUser.name || currentUser.username);
    }

    var save = document.getElementById('save-profile');
    if (save) save.addEventListener('click', async function () {
      var name = nameInput ? nameInput.value.trim() : '';
      var username = withoutAt(userInput ? userInput.value : '');
      var email = emailInput ? emailInput.value.trim() : '';
      if (!name || !username || !email) {
        showToast('Заполните имя, username и почту.');
        return;
      }
      setBusy(save, true, 'Сохраняем…');
      try {
        currentUser = await api.updateProfile({ name: name, username: username, email: email });
        var avatar = document.querySelector('.avatar.xl');
        if (avatar) avatar.textContent = initials(name);
        showToast('Профиль сохранён.');
      } catch (error) {
        showToast(friendlyError(error));
      } finally {
        setBusy(save, false);
      }
    });

    var local = readLocalProfile();
    var localName = document.getElementById('lp-name');
    var localEmail = document.getElementById('lp-email');
    if (localName) localName.value = local.name || '';
    if (localEmail) localEmail.value = local.email || '';
    var saveLocal = document.getElementById('save-local-profile');
    if (saveLocal) saveLocal.addEventListener('click', function () {
      localStorage.setItem(LOCAL_PROFILE_KEY, JSON.stringify({
        name: localName ? localName.value.trim() : '',
        email: localEmail ? localEmail.value.trim() : ''
      }));
      showToast('Сохранено только в этом браузере.');
    });
    var resetLocal = document.getElementById('reset-local-profile');
    if (resetLocal) resetLocal.addEventListener('click', function () {
      localStorage.removeItem(LOCAL_PROFILE_KEY);
      if (localName) localName.value = '';
      if (localEmail) localEmail.value = '';
      showToast('Локальные данные сброшены.');
    });

    var logout = document.getElementById('logout');
    if (logout) logout.addEventListener('click', async function () {
      disconnectWebSocket();
      await api.logout();
      window.location.replace('login.html');
    });
  }

  async function initContacts() {
    var queryInput = document.getElementById('q');
    var list = document.getElementById('filterable');
    var empty = document.getElementById('empty');
    if (!queryInput || !list) return;

    renderSavedContacts();
    var timer = null;

    async function search() {
      // Поиск по @username работает так же, как по обычному username.
      var query = withoutAt(queryInput.value);
      if (!query) {
        list.innerHTML = '';
        if (empty) empty.classList.remove('show');
        return;
      }
      list.innerHTML = '<div class="list-state">Ищем пользователей…</div>';
      try {
        var users = await api.searchUsers(query);
        users = Array.isArray(users) ? users.filter(function (user) { return user.id !== currentUser.id; }) : [];
        if (!users.length) {
          list.innerHTML = '';
          if (empty) empty.classList.add('show');
          return;
        }
        if (empty) empty.classList.remove('show');
        list.innerHTML = users.map(function (user) {
          var label = contactLabel(user);
          rememberAvatar(user.id, user.avatar_url);
          return '<div class="row contact center contact-result" data-user-id="' + escapeHTML(user.id) + '">' +
            '<button class="contact-open" type="button" data-profile-id="' + escapeHTML(user.id) + '" title="Открыть профиль">' +
              avatarSpan(user.id, label, 'avatar') +
              '<span class="meta"><span class="name">' + escapeHTML(label) + '</span><span class="desc">' + escapeHTML(withAt(user.username)) + '</span></span>' +
            '</button></div>';
        }).join('');
      } catch (error) {
        list.innerHTML = '<div class="list-state error-state">' + escapeHTML(friendlyError(error)) + '</div>';
      }
    }

    queryInput.addEventListener('input', function () {
      window.clearTimeout(timer);
      timer = window.setTimeout(search, 300);
    });
    async function handleContactClick(event) {
      var remove = event.target.closest('[data-remove-contact]');
      if (remove) {
        var contacts = readContacts().filter(function (contact) {
          return contact.id !== remove.getAttribute('data-remove-contact');
        });
        saveContacts(contacts);
        renderSavedContacts();
        showToast('Удалено из контактов.');
        return;
      }
      var profile = event.target.closest('[data-profile-id]');
      if (profile) {
        openUserProfile(profile.getAttribute('data-profile-id'));
      }
    }

    list.addEventListener('click', handleContactClick);
    var saved = document.getElementById('saved-contacts');
    if (saved) saved.addEventListener('click', handleContactClick);
  }

  async function resolveChatName(chat) {
    if (chat.type === 'group') return chat.name || 'Групповой чат';
    if (chat.type === 'saved') return chat.name || 'Избранное';
    var fallback = chat.name || 'Личный чат';
    var partnerID = chat.partner_id;
    if (!partnerID) return fallback;
    var contact = contactByID(partnerID);
    if (contact && contact.alias) {
      chat.partnerUsername = contact.username || '';
      chat.partnerAvatar = contact.avatar || '';
      rememberAvatar(partnerID, chat.partnerAvatar);
      return contact.alias;
    }
    var cacheKey = 'bipchat:user-profile:' + partnerID;
    var cached = localStorage.getItem(cacheKey);
    if (cached) {
      try {
        var saved = JSON.parse(cached);
        chat.partnerUsername = saved.username || '';
        chat.partnerAvatar = saved.avatar || '';
        rememberAvatar(partnerID, chat.partnerAvatar);
        if (saved.name) return saved.name;
      } catch (_) {
        // Старый или повреждённый кэш — загружаем профиль заново.
      }
    }
    try {
      var user = await api.getUser(partnerID);
      var label = user.name || user.username;
      if (label) {
        chat.partnerUsername = user.username || '';
        chat.partnerAvatar = user.avatar_url || '';
        rememberAvatar(partnerID, chat.partnerAvatar);
        localStorage.setItem(cacheKey, JSON.stringify({
          name: label,
          username: chat.partnerUsername,
          avatar: chat.partnerAvatar
        }));
        return label;
      }
    } catch (_) {
      // Имя из API недоступно — показываем нейтральный заголовок.
    }
    return fallback;
  }

  async function loadChatList() {
    var chatList = document.getElementById('chat-items');
    if (!chatList) return [];
    chatList.innerHTML = '<div class="list-state">Загружаем чаты…</div>';
    var chats = await api.getChats();
    chats = Array.isArray(chats) ? chats : [];
    var namedChats = await Promise.all(chats.map(async function (chat) {
      chat.displayName = await resolveChatName(chat);
      return chat;
    }));
    namedChats.sort(function (a, b) {
      var aTime = a.last_message_time || a.created_at || '';
      var bTime = b.last_message_time || b.created_at || '';
      return new Date(bTime) - new Date(aTime);
    });
    if (!namedChats.length) {
      chatList.innerHTML = '<div class="list-state">Пока нет чатов. Найдите пользователя в разделе «Контакты», чтобы начать переписку.</div>';
      return namedChats;
    }
    chatList.innerHTML = namedChats.map(function (chat) {
      var id = chat.id;
      var title = chat.displayName;
      var lastMessage = chat.last_message || 'Нет сообщений';
      var time = formatTime(chat.last_message_time || chat.created_at);
      var unread = Number(chat.unread_count || 0);
      var kind = chat.type === 'group' ? 'group' : 'direct';
      var avatarUserID = isPrivateChat(chat) && chat.partner_id ? chat.partner_id : id;
      var avatarMarkup = avatarSpan(avatarUserID, title, 'avatar');
      if (isPrivateChat(chat) && chat.partner_id) {
        avatarMarkup = '<span class="profile-hit" data-profile-id="' + escapeHTML(chat.partner_id) + '" title="Открыть профиль">' + avatarMarkup + '</span>';
      }
      return '<button class="chat-item" type="button" data-chat-id="' + escapeHTML(id) + '" data-kind="' + kind + '">' +
        avatarMarkup +
        (unread ? '<span class="mini-unread">' + escapeHTML(unread) + '</span>' : '') +
        '<span class="body"><span class="top"><span class="nm">' + escapeHTML(title) + '</span><span class="tm">' + escapeHTML(time) + '</span></span>' +
        '<span class="bottom"><span class="pv">' + escapeHTML(lastMessage) + '</span>' + (unread ? '<span class="unread">' + unread + '</span>' : '') + '</span></span></button>';
    }).join('');
    return namedChats;
  }

  function renderMessage(message, arrived) {
    var mine = message.sender_id === currentUser.id;
    var wrapper = document.createElement('div');
    wrapper.className = 'msg' + (mine ? ' me' : '') + (arrived ? ' arrived' : '') +
      (message.is_pinned ? ' pinned' : '') + (selectedMessages[message.id] ? ' selected' : '');
    wrapper.setAttribute('data-message-id', message.id);
    var sender = mine ? (message.sender_name || 'Вы') : senderLabel(message);
    var timestamp = formatTime(message.created_at);

    if (selectionMode) {
      var check = document.createElement('span');
      check.className = 'msg-check';
      check.innerHTML = selectedMessages[message.id] ? icon('check') : '';
      wrapper.appendChild(check);
    }

    if (!mine) {
      var avatar = document.createElement('span');
      avatar.className = 'avatar msg-avatar ' + avatarColor(message.sender_id || sender);
      avatar.innerHTML = avatarInner(message.sender_id, sender);
      if (message.sender_id) {
        avatar.setAttribute('data-profile-id', message.sender_id);
        avatar.title = 'Открыть профиль: ' + sender;
      }
      wrapper.appendChild(avatar);
    }

    var bubble = document.createElement('div');
    bubble.className = 'bubble';
    var quote = '';
    if (message.reply_to_id) {
      var target = currentMessages.find(function (entry) { return entry.id === message.reply_to_id; });
      if (target) {
        quote = '<div class="reply-quote"><span class="rq-name">' + escapeHTML(senderLabel(target)) +
          '</span><span class="rq-text">' + escapeHTML((target.content || '').slice(0, 120)) + '</span></div>';
      }
    }
    bubble.innerHTML = (message.is_pinned ? '<div class="pin-mark">' + icon('pin') + ' Закреплено</div>' : '') +
      (!mine && currentChat && currentChat.type === 'group' ? '<div class="who">' + escapeHTML(sender) + '</div>' : '') +
      quote +
      '<div class="line"><span class="message-text"></span><span class="tm">' + escapeHTML(timestamp) + '</span></div>';
    bubble.querySelector('.message-text').textContent = message.content || '';
    wrapper.appendChild(bubble);
    return wrapper;
  }

  async function loadMessages(chatID, appendOnly) {
    var messageBox = document.getElementById('messages');
    if (!messageBox) return;
    try {
      var messages = await api.getMessages(chatID, 100, 0);
      messages = Array.isArray(messages) ? messages : [];
      // Один и тот же message.id может прийти из POST, WS и polling — рендерим один раз.
      var seen = Object.create(null);
      messages = messages.filter(function (message) {
        if (!message.id || seen[message.id]) return false;
        seen[message.id] = true;
        return true;
      });
      if (appendOnly && messages.length === currentMessages.length &&
          messages.every(function (m, i) { return m.id === currentMessages[i].id; })) return;

      // Подгружаем аватары отправителей один раз и кэшируем их.
      var missing = [];
      messages.forEach(function (message) {
        if (message.sender_id && message.sender_id !== currentUser.id &&
            !Object.prototype.hasOwnProperty.call(userAvatarCache, message.sender_id) &&
            missing.indexOf(message.sender_id) === -1) missing.push(message.sender_id);
      });
      if (missing.length) {
        await Promise.all(missing.map(function (userID) {
          return api.getUser(userID)
            .then(function (user) { rememberAvatar(userID, user.avatar_url); })
            .catch(function () { rememberAvatar(userID, ''); });
        }));
      }

      var shouldStick = messageBox.scrollHeight - messageBox.scrollTop - messageBox.clientHeight < 90;
      var known = Object.create(null);
      currentMessages.forEach(function (message) { known[message.id] = true; });
      var arrived = Object.create(null);
      if (appendOnly) {
        messages.forEach(function (message) { if (!known[message.id]) arrived[message.id] = true; });
      }
      currentMessages = messages;
      messageBox.innerHTML = '';
      var previousDay = '';
      currentMessages.forEach(function (message) {
        var day = formatDay(message.created_at);
        if (day !== previousDay) {
          var divider = document.createElement('div');
          divider.className = 'day';
          divider.textContent = day;
          messageBox.appendChild(divider);
          previousDay = day;
        }
        messageBox.appendChild(renderMessage(message, !!arrived[message.id]));
      });
      if (!messages.length) messageBox.innerHTML = '<div class="message-empty">Сообщений пока нет. Напишите первым.</div>';
      renderPinnedBar();
      if (!appendOnly || shouldStick) messageBox.scrollTop = messageBox.scrollHeight;
      if (api.markAsRead) api.markAsRead(chatID).catch(function () {});
    } catch (error) {
      if (!appendOnly) messageBox.innerHTML = '<div class="list-state error-state">' + escapeHTML(friendlyError(error)) + '</div>';
    }
  }

  function renderChatHeader(chat) {
    var headAvatar = document.getElementById('head-av');
    var headTitle = document.getElementById('head-t');
    var headSub = document.getElementById('head-s');
    if (!headAvatar || !headTitle || !headSub) return;
    var profileButton = document.getElementById('head-profile');
    var headName = chat.displayName || chat.name || 'Чат';
    var avatarUserID = isPrivateChat(chat) && chat.partner_id ? chat.partner_id : chat.id;
    headAvatar.className = 'avatar ' + avatarColor(avatarUserID);
    headAvatar.innerHTML = isPrivateChat(chat) && chat.partner_id
      ? avatarInner(chat.partner_id, headName)
      : escapeHTML(initials(headName));
    headTitle.textContent = headName;
    if (profileButton) {
      if (isPrivateChat(chat) && chat.partner_id) {
        profileButton.classList.add('active');
        profileButton.title = 'Открыть профиль: ' + headName;
      } else {
        profileButton.classList.remove('active');
        profileButton.title = '';
      }
    }
    headSub.textContent = chat.type === 'group' ? 'Групповой чат' : (chat.type === 'saved' ? 'Личные заметки' :
      (chat.partnerUsername ? withAt(chat.partnerUsername) + ' · Личная переписка' : 'Личная переписка'));
  }

  // Псевдоним применяется сразу: шапка, список чатов и уже загруженные сообщения.
  function applyContactAliasToUI(userID, alias) {
    var chats = window.__bipChats || [];
    chats.forEach(function (chat) {
      if (chat.partner_id !== userID) return;
      chat.displayName = alias || chat.name || 'Личный чат';
      var row = document.querySelector('.chat-item[data-chat-id="' + CSS.escape(chat.id) + '"]');
      var name = row && row.querySelector('.nm');
      if (name) name.textContent = chat.displayName;
    });
    if (!currentChat || currentChat.partner_id !== userID) return;
    currentChat.displayName = alias || currentChat.name || 'Личный чат';
    renderChatHeader(currentChat);
    if (document.getElementById('messages')) loadMessages(currentChat.id, false);
  }

  async function openChat(chat, item) {
    clearReply();
    selectionMode = false;
    selectedMessages = Object.create(null);
    updateSelectionBar();
    currentChat = chat;
    document.querySelectorAll('.chat-item').forEach(function (node) { node.classList.toggle('active', node === item); });
    renderChatHeader(chat);
    var pinned = document.getElementById('pinned');
    if (pinned) pinned.classList.add('hidden');
    document.querySelector('.app.wide').classList.add('mobile-chat-open');
    localStorage.setItem('bipchat:selected-chat', chat.id);
    await loadMessages(chat.id, false);
    if (messagePoll) window.clearInterval(messagePoll);
    messagePoll = window.setInterval(function () {
      if (currentChat && currentChat.id === chat.id) loadMessages(chat.id, true);
    }, 4000);
  }

  function initChatList() {
    var search = document.getElementById('chat-search');
    var list = document.getElementById('chat-items');
    if (!list) return;
    function apply() {
      var query = search ? search.value.trim().toLowerCase() : '';
      list.querySelectorAll('.chat-item').forEach(function (item) {
        item.hidden = !!query && item.textContent.toLowerCase().indexOf(query) === -1;
      });
    }
    if (search) search.addEventListener('input', apply);
    list.addEventListener('click', function (event) {
      var profile = event.target.closest('[data-profile-id]');
      if (profile) {
        event.stopPropagation();
        openUserProfile(profile.getAttribute('data-profile-id'));
        return;
      }
      var item = event.target.closest('[data-chat-id]');
      if (!item) return;
      var id = item.getAttribute('data-chat-id');
      var chat = window.__bipChats.find(function (entry) { return entry.id === id; });
      if (chat) openChat(chat, item);
    });
  }

  async function refreshChatList() {
    var list = document.getElementById('chat-items');
    if (!list) return;
    try {
      window.__bipChats = await loadChatList();
      if (currentChat) {
        var active = list.querySelector('[data-chat-id="' + CSS.escape(currentChat.id) + '"]');
        if (active) active.classList.add('active');
      }
    } catch (_) {
      // Polling повторит попытку позже.
    }
  }

  function scheduleChatListRefresh() {
    window.clearTimeout(chatListRefresh);
    chatListRefresh = window.setTimeout(refreshChatList, 500);
  }

  function initSidebarResize() {
    var app = document.querySelector('.app.wide');
    var resizer = document.getElementById('sidebar-resizer');
    var toggle = document.getElementById('sidebar-toggle');
    var searchButton = document.getElementById('chat-search-btn');
    var searchInput = document.getElementById('chat-search');
    if (!app || !resizer || !window.matchMedia || !window.matchMedia('(min-width: 861px)').matches) return;

    var COLLAPSED = 76;
    var MIN = 240;
    var MAX = 560;
    var savedWidth = Number(localStorage.getItem('bipchat:chats-width'));
    var width = savedWidth >= MIN && savedWidth <= MAX ? savedWidth : 280;
    var collapsed = localStorage.getItem('bipchat:chats-collapsed') === '1';

    function apply() {
      app.classList.toggle('chats-collapsed', collapsed);
      app.style.setProperty('--chats-width', width + 'px');
      if (toggle) {
        var label = collapsed ? 'Развернуть список чатов' : 'Свернуть список чатов';
        toggle.setAttribute('title', label);
        toggle.setAttribute('aria-label', label);
      }
      resizer.setAttribute('aria-valuenow', String(collapsed ? COLLAPSED : width));
    }

    function setCollapsed(value) {
      collapsed = value;
      localStorage.setItem('bipchat:chats-collapsed', value ? '1' : '0');
      apply();
    }

    apply();

    if (toggle) toggle.addEventListener('click', function () { setCollapsed(!collapsed); });
    if (searchButton) searchButton.addEventListener('click', function () {
      if (collapsed) setCollapsed(false);
      if (searchInput) window.setTimeout(function () { searchInput.focus(); }, 0);
    });
    resizer.addEventListener('dblclick', function () { setCollapsed(!collapsed); });

    resizer.addEventListener('pointerdown', function (event) {
      if (event.button !== 0) return;
      event.preventDefault();
      var startX = event.clientX;
      var startWidth = collapsed ? COLLAPSED : width;
      document.body.classList.add('resizing');

      function onMove(moveEvent) {
        var next = Math.max(COLLAPSED, Math.min(MAX, startWidth + moveEvent.clientX - startX));
        if (next <= COLLAPSED + 50) {
          setCollapsed(true);
          return;
        }
        if (collapsed) setCollapsed(false);
        width = Math.max(MIN, next);
        app.style.setProperty('--chats-width', width + 'px');
      }

      function onUp() {
        document.removeEventListener('pointermove', onMove);
        document.removeEventListener('pointerup', onUp);
        document.removeEventListener('pointercancel', onUp);
        document.body.classList.remove('resizing');
        localStorage.setItem('bipchat:chats-width', String(width));
      }

      document.addEventListener('pointermove', onMove);
      document.addEventListener('pointerup', onUp);
      document.addEventListener('pointercancel', onUp);
    });
  }

  function hideContextMenu() {
    if (contextMenu) contextMenu.classList.remove('open');
  }

  var ICON_FILES = {
    reply: 'corner-up-left', pin: 'pin', unpin: 'pin-off', copy: 'copy',
    forward: 'forward', del: 'trash-2', select: 'list-checks', check: 'check'
  };

  function icon(name) {
    return '<span class="ico ico-' + (ICON_FILES[name] || name) + '" aria-hidden="true"></span>';
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text);
    return new Promise(function (resolve, reject) {
      var area = document.createElement('textarea');
      area.value = text;
      area.style.position = 'fixed';
      area.style.opacity = '0';
      document.body.appendChild(area);
      area.select();
      try {
        if (document.execCommand('copy')) resolve();
        else reject(new Error('copy failed'));
      } catch (error) {
        reject(error);
      } finally {
        document.body.removeChild(area);
      }
    });
  }

  function refreshMessages() {
    if (currentChat) return loadMessages(currentChat.id, false);
    return Promise.resolve();
  }

  function clearReply() {
    replyTarget = null;
    var bar = document.getElementById('reply-bar');
    if (bar) bar.classList.add('hidden');
  }

  function setReplyTarget(message) {
    replyTarget = message;
    var bar = document.getElementById('reply-bar');
    if (!bar) return;
    bar.classList.remove('hidden');
    document.getElementById('reply-name').textContent = 'Ответ: ' + senderLabel(message);
    document.getElementById('reply-text').textContent = (message.content || '').slice(0, 120);
    var input = document.getElementById('composer-input');
    if (input) input.focus();
  }

  function updateSelectionBar() {
    var ids = Object.keys(selectedMessages).filter(function (id) { return selectedMessages[id]; });
    var bar = document.getElementById('selection-bar');
    if (bar) bar.classList.toggle('hidden', !selectionMode);
    var count = document.getElementById('selection-count');
    if (count) count.textContent = 'Выбрано: ' + ids.length;
    document.querySelectorAll('.msg').forEach(function (node) {
      var id = node.getAttribute('data-message-id');
      node.classList.toggle('selected', !!selectedMessages[id]);
      var check = node.querySelector('.msg-check');
      if (check) check.innerHTML = selectedMessages[id] ? icon('check') : '';
    });
  }

  function setSelectionMode(value) {
    selectionMode = value;
    if (!value) selectedMessages = Object.create(null);
    updateSelectionBar();
    refreshMessages();
  }

  function renderPinnedBar() {
    var bar = document.getElementById('pinned');
    if (!bar) return;
    var pinned = currentMessages.filter(function (message) { return message.is_pinned; });
    if (!pinned.length) {
      bar.classList.add('hidden');
      bar.innerHTML = '';
      return;
    }
    var message = pinned[pinned.length - 1];
    bar.classList.remove('hidden');
    bar.innerHTML = '<span class="pin-icon">' + icon('pin') + '</span><span class="pin-copy"><strong>' +
      escapeHTML(senderLabel(message)) + '</strong> ' +
      escapeHTML((message.content || '').slice(0, 100)) + '</span>' +
      '<button class="icon-btn" type="button" id="pin-goto" aria-label="Перейти к сообщению"><svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg></button>';
    var goto = document.getElementById('pin-goto');
    if (goto) goto.addEventListener('click', function () {
      var node = document.querySelector('.msg[data-message-id="' + CSS.escape(message.id) + '"]');
      if (node) node.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
  }

  function openForwardModal(message) {
    var chats = window.__bipChats || [];
    if (!chats.length) {
      showToast('Нет чатов для пересылки.');
      return;
    }
    var modal = document.getElementById('forward-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'forward-modal';
      modal.className = 'modal-overlay';
      modal.innerHTML = '<div class="modal-card forward-card">' +
        '<div class="modal-head"><h2>Переслать сообщение</h2>' +
        '<button class="icon-btn" type="button" id="forward-close" aria-label="Закрыть"><svg viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12"/></svg></button></div>' +
        '<p class="about-text" id="forward-preview"></p>' +
        '<div class="forward-list" id="forward-list"></div></div>';
      document.body.appendChild(modal);
      modal.addEventListener('click', function (event) { if (event.target === modal) modal.classList.remove('open'); });
      document.getElementById('forward-close').addEventListener('click', function () { modal.classList.remove('open'); });
      document.getElementById('forward-list').addEventListener('click', async function (event) {
        var button = event.target.closest('[data-forward-chat]');
        if (!button) return;
        var message = modal._message;
        if (!message) return;
        setBusy(button, true, '…');
        try {
          var text = 'Переслано от ' + senderLabel(message) + ':\n' + (message.content || '');
          var targetID = button.getAttribute('data-forward-chat');
          await api.sendMessage(targetID, text);
          modal.classList.remove('open');
          showToast('Сообщение переслано.');
          scheduleChatListRefresh();
          if (currentChat && targetID === currentChat.id) refreshMessages();
        } catch (error) {
          showToast(friendlyError(error));
        } finally {
          setBusy(button, false);
        }
      });
    }
    modal._message = message;
    document.getElementById('forward-preview').textContent = senderLabel(message) + ': ' + (message.content || '').slice(0, 160);
    document.getElementById('forward-list').innerHTML = chats.map(function (chat) {
      return '<button class="forward-chat" type="button" data-forward-chat="' + escapeHTML(chat.id) + '">' +
        avatarSpan(chat.partner_id || chat.id, chat.displayName || chat.name || 'Чат', 'avatar') +
        '<span>' + escapeHTML(chat.displayName || chat.name || 'Чат') + '</span></button>';
    }).join('');
    modal.classList.add('open');
  }

  function showContextMenu(x, y, message) {
    hideContextMenu();
    if (!contextMenu) {
      contextMenu = document.createElement('div');
      contextMenu.id = 'context-menu';
      contextMenu.className = 'context-menu';
      contextMenu.setAttribute('role', 'menu');
      contextMenu.innerHTML =
        '<button type="button" data-action="reply">' + icon('reply') + 'Ответить</button>' +
        '<button type="button" data-action="pin"><span class="pin-icon">' + icon('pin') + '</span><span class="pin-label">Закрепить</span></button>' +
        '<button type="button" data-action="copy">' + icon('copy') + 'Скопировать</button>' +
        '<button type="button" data-action="forward">' + icon('forward') + 'Переслать</button>' +
        '<button type="button" data-action="delete" class="danger">' + icon('del') + 'Удалить</button>' +
        '<div class="ctx-divider"></div>' +
        '<button type="button" data-action="select">' + icon('select') + 'Выделить</button>';
      document.body.appendChild(contextMenu);
      contextMenu.addEventListener('click', async function (event) {
        var button = event.target.closest('[data-action]');
        if (!button) return;
        var msg = contextMenu._message;
        hideContextMenu();
        if (!msg || !currentChat) return;
        var action = button.getAttribute('data-action');
        try {
          if (action === 'reply') setReplyTarget(msg);
          else if (action === 'copy') {
            await copyText(msg.content || '');
            showToast('Скопировано.');
          } else if (action === 'pin') {
            await api.togglePin(currentChat.id, msg.id);
            showToast(msg.is_pinned ? 'Откреплено.' : 'Закреплено.');
            await refreshMessages();
            scheduleChatListRefresh();
          } else if (action === 'forward') openForwardModal(msg);
          else if (action === 'delete') {
            if (msg.sender_id !== currentUser.id) {
              showToast('Удалять можно только свои сообщения.');
              return;
            }
            await api.deleteMessage(currentChat.id, msg.id);
            showToast('Сообщение удалено.');
            await refreshMessages();
            scheduleChatListRefresh();
          } else if (action === 'select') {
            selectionMode = true;
            selectedMessages[msg.id] = true;
            updateSelectionBar();
            refreshMessages();
          }
        } catch (error) {
          showToast(friendlyError(error));
        }
      });
    }
    contextMenu._message = message;
    var deleteButton = contextMenu.querySelector('[data-action="delete"]');
    if (deleteButton) deleteButton.hidden = message.sender_id !== currentUser.id;
    var pinLabel = contextMenu.querySelector('.pin-label');
    if (pinLabel) pinLabel.textContent = message.is_pinned ? 'Открепить' : 'Закрепить';
    var pinIcon = contextMenu.querySelector('[data-action="pin"] .pin-icon');
    if (pinIcon) pinIcon.innerHTML = icon(message.is_pinned ? 'unpin' : 'pin');
    var width = 210;
    var height = 270;
    contextMenu.style.left = Math.max(8, Math.min(x, window.innerWidth - width - 8)) + 'px';
    contextMenu.style.top = Math.max(8, Math.min(y, window.innerHeight - height - 8)) + 'px';
    contextMenu.classList.add('open');
  }

  function setupMessageActions() {
    var messageBox = document.getElementById('messages');
    if (!messageBox) return;

    messageBox.addEventListener('contextmenu', function (event) {
      var node = event.target.closest('[data-message-id]');
      if (!node) return;
      event.preventDefault();
      var message = currentMessages.find(function (item) { return item.id === node.getAttribute('data-message-id'); });
      if (message) showContextMenu(event.clientX, event.clientY, message);
    });

    document.addEventListener('click', function (event) {
      if (contextMenu && !contextMenu.contains(event.target)) hideContextMenu();
      var forward = document.getElementById('forward-modal');
      if (forward && event.target === forward) forward.classList.remove('open');
    });
    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') {
        hideContextMenu();
        var forward = document.getElementById('forward-modal');
        if (forward) forward.classList.remove('open');
      }
    });
    window.addEventListener('resize', hideContextMenu);
    messageBox.addEventListener('scroll', hideContextMenu);

    var replyCancel = document.getElementById('reply-cancel');
    if (replyCancel) replyCancel.addEventListener('click', clearReply);

    var selectionCancel = document.getElementById('selection-cancel');
    if (selectionCancel) selectionCancel.addEventListener('click', function () { setSelectionMode(false); });
    var selectionCopy = document.getElementById('selection-copy');
    if (selectionCopy) selectionCopy.addEventListener('click', async function () {
      var texts = currentMessages.filter(function (message) { return selectedMessages[message.id]; })
        .map(function (message) { return message.content || ''; });
      if (!texts.length) { showToast('Выберите сообщения.'); return; }
      try {
        await copyText(texts.join('\n'));
        showToast('Скопировано.');
      } catch (error) { showToast(friendlyError(error)); }
    });
    var selectionDelete = document.getElementById('selection-delete');
    if (selectionDelete) selectionDelete.addEventListener('click', async function () {
      var chosen = currentMessages.filter(function (message) { return selectedMessages[message.id]; });
      if (!chosen.length) { showToast('Выберите сообщения.'); return; }
      var button = this;
      setBusy(button, true, 'Удаляем…');
      var deleted = 0;
      var skipped = 0;
      try {
        for (var i = 0; i < chosen.length; i++) {
          if (chosen[i].sender_id !== currentUser.id) { skipped++; continue; }
          await api.deleteMessage(currentChat.id, chosen[i].id);
          deleted++;
        }
        setSelectionMode(false);
        scheduleChatListRefresh();
        showToast(skipped ? ('Удалено: ' + deleted + '. Чужие сообщения пропущены.') : ('Удалено: ' + deleted + '.'));
      } catch (error) {
        showToast(friendlyError(error));
      } finally {
        setBusy(button, false);
      }
    });
  }

  function connectWebSocket() {
    var token = api.getToken();
    if (!token || !window.WebSocket) return;
    var scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url = scheme + '//' + window.location.host + '/ws?token=' + encodeURIComponent(token);
    try {
      wsSocket = new WebSocket(url);
    } catch (_) {
      return;
    }
    wsSocket.onmessage = function (event) {
      var data;
      try { data = JSON.parse(event.data); } catch (_) { return; }
      if (!data || !data.payload || !data.payload.chat_id) return;
      if (data.type !== 'new_message' && data.type !== 'message_deleted' && data.type !== 'message_pinned') return;
      scheduleChatListRefresh();
      if (data.type === 'new_message') {
        notifyIncomingMessage(data.payload);
      } else {
        var payload = data.payload;
        if (data.type === 'message_deleted') {
          currentMessages = currentMessages.filter(function (message) { return message.id !== payload.message_id; });
          if (selectedMessages[payload.message_id]) delete selectedMessages[payload.message_id];
          if (replyTarget && replyTarget.id === payload.message_id) clearReply();
        } else if (currentChat && payload.chat_id === currentChat.id) {
          currentMessages.forEach(function (message) {
            if (message.id === payload.message_id) message.is_pinned = !!payload.is_pinned;
          });
        }
      }
      if (currentChat && data.payload.chat_id === currentChat.id) {
        loadMessages(currentChat.id, data.type === 'new_message');
      }
    };
    wsSocket.onerror = function () {
      if (wsSocket) wsSocket.close();
    };
    wsSocket.onclose = function () {
      wsSocket = null;
      window.clearTimeout(wsReconnect);
      // Polling продолжает работать, пока WS недоступен.
      wsReconnect = window.setTimeout(connectWebSocket, 3000);
    };
  }

  function disconnectWebSocket() {
    window.clearTimeout(wsReconnect);
    if (wsSocket) {
      var socket = wsSocket;
      wsSocket = null;
      socket.onclose = null;
      socket.close();
    }
  }

  async function initMessages() {
    var list = document.getElementById('chat-items');
    if (!list) return;
    initChatList();
    initSidebarResize();
    connectWebSocket();
    setupMessageActions();
    var messageBox = document.getElementById('messages');
    if (messageBox) messageBox.addEventListener('click', function (event) {
      if (selectionMode) {
        var node = event.target.closest('[data-message-id]');
        if (!node) return;
        var id = node.getAttribute('data-message-id');
        selectedMessages[id] = !selectedMessages[id];
        updateSelectionBar();
        return;
      }
      var hit = event.target.closest('[data-profile-id]');
      if (hit) openUserProfile(hit.getAttribute('data-profile-id'));
    });
    var profileButton = document.getElementById('head-profile');
    if (profileButton) profileButton.addEventListener('click', function () {
      if (isPrivateChat(currentChat) && currentChat.partner_id) {
        openUserProfile(currentChat.partner_id);
      }
    });
    var back = document.getElementById('chat-back');
    if (back) back.addEventListener('click', function () {
      document.querySelector('.app.wide').classList.remove('mobile-chat-open');
      localStorage.removeItem('bipchat:selected-chat');
      currentChat = null;
      if (messagePoll) window.clearInterval(messagePoll);
    });

    var chats = await loadChatList();
    window.__bipChats = chats;
    var selectedID = localStorage.getItem('bipchat:selected-chat');
    var selected = chats.find(function (chat) { return chat.id === selectedID; });
    if (!selected && window.matchMedia('(min-width: 861px)').matches) selected = chats[0];
    if (selected) {
      var selectedElement = list.querySelector('[data-chat-id="' + CSS.escape(selected.id) + '"]');
      await openChat(selected, selectedElement);
    } else {
      document.querySelector('.app.wide').classList.remove('mobile-chat-open');
      document.getElementById('head-t').textContent = 'Выберите чат';
      document.getElementById('head-s').textContent = 'Ваши сообщения появятся здесь';
      document.getElementById('messages').innerHTML = '<div class="message-empty">Выберите существующий чат или найдите пользователя в контактах.</div>';
    }

    var form = document.getElementById('composer-form');
    var input = document.getElementById('composer-input');
    if (form && input) {
      function resizeComposer() {
        input.style.height = 'auto';
        input.style.height = Math.min(input.scrollHeight, 140) + 'px';
      }
      input.addEventListener('input', resizeComposer);
      input.addEventListener('keydown', function (event) {
        if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
          event.preventDefault();
          form.requestSubmit();
        }
      });
      resizeComposer();

      form.addEventListener('submit', async function (event) {
        event.preventDefault();
        var content = input.value.trim();
        if (!content || !currentChat) return;
        var submit = form.querySelector('button[type="submit"]');
        setBusy(submit, true, '…');
        try {
          await api.sendMessage(currentChat.id, content, replyTarget && replyTarget.id);
          input.value = '';
          resizeComposer();
          clearReply();
          await loadMessages(currentChat.id, false);
          await refreshChatList();
        } catch (error) {
          showToast(friendlyError(error));
        } finally {
          setBusy(submit, false);
          input.focus();
        }
      });
    }
  }

  async function initGroups() {
    var list = document.getElementById('group-list');
    if (!list) return;
    list.innerHTML = '<div class="list-state">Загружаем группы…</div>';
    try {
      var chats = await api.getChats();
      var groups = (Array.isArray(chats) ? chats : []).filter(function (chat) { return chat.type === 'group'; });
      if (!groups.length) {
        list.innerHTML = '<div class="list-state">Групповых чатов пока нет.</div>';
        return;
      }
      list.innerHTML = groups.map(function (chat) {
        var title = chat.name || 'Групповой чат';
        return '<button class="row group" type="button" data-chat-id="' + escapeHTML(chat.id) + '">' +
          '<span class="avatar ' + avatarColor(chat.id) + '">' + escapeHTML(initials(title)) + '</span>' +
          '<span class="meta"><span class="name">' + escapeHTML(title) + '</span><span class="desc">Групповой чат</span></span>' +
          '<span class="side">Открыть</span></button>';
      }).join('');
      list.addEventListener('click', function (event) {
        var row = event.target.closest('[data-chat-id]');
        if (!row) return;
        localStorage.setItem('bipchat:selected-chat', row.getAttribute('data-chat-id'));
        window.location.href = 'messages.html';
      });
    } catch (error) {
      list.innerHTML = '<div class="list-state error-state">' + escapeHTML(friendlyError(error)) + '</div>';
    }
  }

  function initFiles() {
    var list = document.getElementById('file-list');
    if (!list) return;
    list.innerHTML = '<div class="list-state">Файлов пока нет. В текущем backend доступна загрузка аватара, но общий список вложений ещё не подключён.</div>';
  }

  async function bootstrap() {
    applySettings(readSettings());
    renderNavigation();
    if (document.getElementById('auth-tabs')) {
      await initAuthPage();
      return;
    }
    var allowed = await requireAuth();
    if (!allowed) return;
    renderNavigation();

    if (document.getElementById('chat-items')) await initMessages();
    if (document.getElementById('filterable')) await initContacts();
    if (document.getElementById('group-list')) await initGroups();
    if (document.getElementById('file-list')) initFiles();
    if (document.querySelector('.settings')) initSettings();
  }

  document.addEventListener('DOMContentLoaded', function () {
    bootstrap().catch(function (error) {
      console.error('BipChat bootstrap error:', error);
      window.showToast(friendlyError(error));
    });
  });
})();
