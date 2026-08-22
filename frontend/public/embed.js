  import { streamErrorPresentation } from '/embed-status.js';

  const p = new URLSearchParams(location.search);
  const src = p.get('src');
  const label = p.get('label') || 'Kamera';
  const mode = p.get('mode') || 'mse,mp4,mjpeg';
  let muted = true;
  document.documentElement.style.setProperty('--fit', p.get('fit') === 'contain' ? 'contain' : 'cover');
  if (src) {
    await import('/go2rtc/video-stream.js'); // registriert <video-stream> same-origin
    const el = document.createElement('video-stream');
    el.background = true;            // Stream nicht pausieren, wenn iframe versteckt
    el.muted = true;
    el.mode = mode;
    el.src = '/go2rtc/api/ws?src=' + encodeURIComponent(src);
    document.body.appendChild(el);

    const notice = document.createElement('div');
    notice.className = 'stream-error';
    notice.hidden = true;
    notice.setAttribute('role', 'status');
    notice.innerHTML = '<div class="stream-error-card"><div class="stream-error-mark" aria-hidden="true"></div><div class="stream-error-label"></div><div class="stream-error-title"></div><div class="stream-error-detail"></div></div>';
    notice.querySelector('.stream-error-label').textContent = label;
    document.body.appendChild(notice);

    // Native Controls cross-browser zuverlässig abschalten (auch Firefox):
    let readySent = false;
    const notifyReady = () => {
      if (readySent) return;
      readySent = true;
      notice.hidden = true;
      window.parent?.postMessage({ type: 'camera-stream-ready', src }, location.origin);
    };
    const syncErrorNotice = () => {
      const playerMode = el.querySelector?.('.mode')?.textContent?.trim().toLowerCase() || '';
      if (playerMode !== 'error') {
        if (readySent) notice.hidden = true;
        return;
      }
      const raw = el.querySelector?.('.status')?.textContent?.trim() || '';
      const presentation = streamErrorPresentation(raw);
      const title = notice.querySelector('.stream-error-title');
      const detail = notice.querySelector('.stream-error-detail');
      if (title.textContent !== presentation.title) title.textContent = presentation.title;
      if (detail.textContent !== presentation.detail) detail.textContent = presentation.detail;
      notice.hidden = false;
    };
    const strip = () => {
      const v = el.video || el.querySelector?.('video');
      if (v) {
        v.controls = false;
        v.muted = muted;
        v.removeAttribute('controls');
        v.style.pointerEvents = 'none';
        if (!muted) v.play?.().catch?.(() => undefined);
        if (v.readyState >= 2) notifyReady();
        v.addEventListener('loadeddata', notifyReady, { once: true });
        v.addEventListener('playing', notifyReady, { once: true });
      }
      for (const node of el.querySelectorAll?.('*') || []) {
        if (node.textContent?.trim() === 'MSE') node.style.display = 'none';
      }
      syncErrorNotice();
    };
    strip();
    new MutationObserver(strip).observe(document.body, { childList: true, subtree: true });
    window.addEventListener('message', (event) => {
      if (event.origin !== location.origin || event.data?.type !== 'camera-audio') return;
      muted = event.data.muted !== false;
      el.muted = muted;
      strip();
    });
  }
