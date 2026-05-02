(function () {
    var btn = document.getElementById('theme-toggle');
    if (!btn) return;

    function getTheme() {
        var stored = localStorage.getItem('theme');
        if (stored === 'dark' || stored === 'light') return stored;
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }

    function applyTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        btn.setAttribute('aria-label', theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme');
        btn.querySelector('.icon-sun').style.display = theme === 'dark' ? 'inline' : 'none';
        btn.querySelector('.icon-moon').style.display = theme === 'dark' ? 'none' : 'inline';
    }

    applyTheme(getTheme());

    btn.addEventListener('click', function () {
        var current = document.documentElement.getAttribute('data-theme') || 'light';
        var next = current === 'dark' ? 'light' : 'dark';
        localStorage.setItem('theme', next);
        applyTheme(next);
    });
})();
