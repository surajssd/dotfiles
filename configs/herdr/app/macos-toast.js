import Toast from "./bundled-toast.js";
import { appHome, checking, exec, props } from "herdr";

function shellQuote(value) {
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

export default class MacOSToast extends Toast {
  init(viewProps, cx) {
    super.init?.(viewProps, cx);
    if (checking()) return;
    const alert = props();
    if (alert?.action?.name !== "agent.focus") return;

    const appDir = appHome();
    const modDir = `${appDir}/mods/dotfiles-macos-toast`;
    const focusArgs = JSON.stringify({ terminal: alert.action.args.terminal });
    const onClick = `HERDR_APP_HOME=${shellQuote(appDir)} ${shellQuote(modDir + "/herdr-app")} do agent.focus ${shellQuote(focusArgs)}`;

    cx.spawn(async () => {
      // terminal-notifier strips one leading backslash after parsing option values.
      const result = await exec(`${modDir}/terminal-notifier`, [
        "-title", "Herdr",
        "-subtitle", "\\" + alert.title,
        "-message", "\\" + (alert.body ?? ""),
        "-activate", "dev.herdr.app",
        "-execute", onClick,
      ]);
      if (result.code !== 0) throw new Error(result.stderr || "Could not post the Herdr notification");
    });
  }
}
