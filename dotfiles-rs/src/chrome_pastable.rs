use arboard::Clipboard;
use std::env;

pub fn chrome() {
    let code = "var allowPaste = function(e){
  e.stopImmediatePropagation();
  return true;
};

document.addEventListener('paste', allowPaste, true);";
    let mut clipboard = Clipboard::new().unwrap();
    clipboard.set_text(code).unwrap();

    let shortcut = "Ctrl + Shift + I";
    let osx_shortcut = "⌘ + ⌥ + I";

    // Detect what OS is this?
    let os = env::consts::OS;
    let shortcut = match os {
        "macos" => osx_shortcut,
        _ => shortcut,
    };

    println!("The code to make the page pastable is copied to the clipboard.");
    println!("Go to browser and press {} to open 'Developer Tools' go to 'Console' and paste the code in clipboard.", shortcut);
}
