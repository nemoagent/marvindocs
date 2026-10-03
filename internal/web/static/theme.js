// Applies the saved (or OS-preferred) theme before first paint. Loaded
// synchronously from <head> so there is no light/dark flash; the toggle
// itself lives in app.js.
(function () {
    var t = localStorage.getItem('theme');
    if (!t) t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', t);
})();
