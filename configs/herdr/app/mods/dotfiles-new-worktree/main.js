import { View, div } from "gpui-kit";
import { checking, spaces, panes, props, openOverlay, closeOverlay } from "herdr";

export default class NewWorktree extends View {
  init(_props, cx) {
    if (checking()) return;
    cx.timer.after(0, () => {
      const space = spaces().find(s => s.active);
      const folder = panes().find(p => p.focused)?.cwd;
      closeOverlay(props().overlay);
      if (!space) return;
      openOverlay({
        mod: "worktree",
        kind: "dialog",
        width: 460,
        height: 280,
        props: { space: space.id, folder, action: "new" }
      });
    });
  }

  render() {
    return div();
  }
}
