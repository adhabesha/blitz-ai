const { app, BrowserWindow, shell, Menu } = require("electron");
const path = require("path");
const BASE = (process.env.BLITZ_URL || "https://blitz-ai-5ow4.onrender.com").replace(/\/$/, "");
let win;

function create() {
  win = new BrowserWindow({
    width: 1320, height: 860, minWidth: 420, minHeight: 640,
    backgroundColor: "#050b1f", autoHideMenuBar: true, show: false, title: "Blitz AI",
    icon: path.join(__dirname, "build", "icon.png"),
    webPreferences: { contextIsolation: true, sandbox: true }
  });
  win.webContents.setUserAgent(win.webContents.getUserAgent() + " BlitzDesktop");
  win.once("ready-to-show", () => win.show());
  win.loadURL(BASE + "/app");
  win.webContents.on("did-fail-load", (e, code, desc, url, isMain) => {
    if (isMain && code !== -3) win.loadFile(path.join(__dirname, "offline.html"), { query: { u: BASE + "/app" } });
  });
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith(BASE)) return { action: "allow" };
    shell.openExternal(url);
    return { action: "deny" };
  });
  Menu.setApplicationMenu(Menu.buildFromTemplate([{
    label: "Blitz AI",
    submenu: [
      { label: "Dashboard", accelerator: "CmdOrCtrl+D", click: () => win.loadURL(BASE + "/dashboard") },
      { label: "Admin console", accelerator: "CmdOrCtrl+Shift+A", click: () => win.loadURL(BASE + "/admin") },
      { type: "separator" }, { role: "reload" }, { role: "togglefullscreen" }, { role: "quit" }
    ]
  }]));
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => { if (win) { if (win.isMinimized()) win.restore(); win.focus(); } });
  app.whenReady().then(create);
  app.on("window-all-closed", () => app.quit());
}
