const BACKEND_URL = "http://127.0.0.1:7007";

function probeBackend(): Promise<boolean> {
  return new Promise((resolve) => {
    const img = new Image();
    const timer = setTimeout(() => {
      img.src = "";
      resolve(false);
    }, 1000);
    img.onload = () => {
      clearTimeout(timer);
      resolve(true);
    };
    img.onerror = () => {
      clearTimeout(timer);
      resolve(false);
    };
    img.src = `${BACKEND_URL}/favicon.svg?t=${Date.now()}`;
  });
}

async function waitForBackend(timeoutMs = 30_000): Promise<boolean> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (await probeBackend()) return true;
    await new Promise((resolve) => setTimeout(resolve, 400));
  }
  return false;
}

(async () => {
  const ready = await waitForBackend();
  if (ready) {
    window.location.replace(`${BACKEND_URL}/`);
  } else {
    const app = document.getElementById("app");
    if (app) {
      app.innerHTML = `
        <div class="loader">
          <p class="error">Backend failed to start within 30 seconds.</p>
          <p><small>Check Console.app for logs.</small></p>
        </div>
      `;
    }
  }
})();
