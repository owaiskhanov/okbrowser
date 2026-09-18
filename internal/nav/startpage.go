package nav

// StartHTML is OK Browser's built-in start page. It is a single, dependency
// free document rendered instantly from memory - no network requests, so the
// browser is usable the moment the window opens.
const StartHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>OK Browser</title>
<style>
  :root {
    --bg: #fafafa; --fg: #202124; --muted: #5f6368;
    --brand: #1a73e8; --card: #ffffff; --border: #e3e3e3;
  }
  * { box-sizing: border-box; }
  html, body { height: 100%; }
  body {
    margin: 0; font-family: "Segoe UI", system-ui, Arial, sans-serif;
    background: var(--bg); color: var(--fg);
    display: flex; align-items: center; justify-content: center;
  }
  main { width: min(660px, 92vw); text-align: center; padding: 40px 0; }
  .logo { font-size: 46px; font-weight: 700; letter-spacing: -1.5px; }
  .logo b { color: var(--brand); }
  .tag { color: var(--muted); margin: 10px 0 38px; font-size: 14px; }
  form { display: flex; gap: 10px; }
  form input {
    flex: 1; font-size: 16px; padding: 14px 22px;
    border: 1px solid var(--border); border-radius: 24px;
    outline: none; background: var(--card);
    transition: border-color .15s, box-shadow .15s;
  }
  form input:focus {
    border-color: var(--brand);
    box-shadow: 0 1px 6px rgba(26,115,232,.28);
  }
  form button {
    font-size: 15px; font-weight: 600; color: #fff;
    background: var(--brand); border: 0; border-radius: 22px;
    padding: 14px 28px; cursor: pointer; transition: background .15s;
  }
  form button:hover { background: #1765cc; }
  .links {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: 14px; margin-top: 46px;
  }
  .tile {
    display: flex; flex-direction: column; align-items: center; gap: 10px;
    padding: 18px 8px; background: var(--card);
    border: 1px solid var(--border); border-radius: 14px;
    text-decoration: none; color: var(--fg); font-size: 13px;
    transition: border-color .15s, transform .15s, box-shadow .15s;
  }
  .tile:hover {
    border-color: var(--brand); transform: translateY(-2px);
    box-shadow: 0 4px 14px rgba(0,0,0,.08);
  }
  .dot {
    width: 38px; height: 38px; border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    font-weight: 700; font-size: 16px; color: #fff;
  }
  @media (max-width: 560px) { .links { grid-template-columns: repeat(2, 1fr); } }
</style>
</head>
<body>
<main>
  <div class="logo"><b>OK</b> Browser</div>
  <div class="tag">Fast &amp; light. Search the web or enter an address.</div>
  <form id="f">
    <input id="q" placeholder="Search or type a web address" autofocus autocomplete="off" spellcheck="false">
    <button type="submit">Search</button>
  </form>
  <div class="links">
    <a class="tile" href="https://www.google.com"><span class="dot" style="background:#4285F4">G</span>Google</a>
    <a class="tile" href="https://www.youtube.com"><span class="dot" style="background:#FF0000">&#9654;</span>YouTube</a>
    <a class="tile" href="https://github.com"><span class="dot" style="background:#24292f">G</span>GitHub</a>
    <a class="tile" href="https://mail.google.com"><span class="dot" style="background:#EA4335">M</span>Gmail</a>
    <a class="tile" href="https://www.wikipedia.org"><span class="dot" style="background:#636466">W</span>Wikipedia</a>
    <a class="tile" href="https://www.reddit.com"><span class="dot" style="background:#FF4500">r</span>Reddit</a>
    <a class="tile" href="https://x.com"><span class="dot" style="background:#000000">X</span>X</a>
    <a class="tile" href="https://www.facebook.com"><span class="dot" style="background:#1877F2">f</span>Facebook</a>
  </div>
</main>
<script>
  document.getElementById("f").addEventListener("submit", function (e) {
    e.preventDefault();
    var q = document.getElementById("q").value.trim();
    if (!q) return;
    // Let the host application parse it (same logic as the address bar).
    window.__ok({ t: "go", u: q });
  });
</script>
</body>
</html>`
