use chrono::Utc;
use shellexpand;
use std::path::Path;
use std::process::Command;

pub const DUMP_LOCATION: &str = "$HOME/code/work/dump";

pub fn dump(location: &str) {
    // The file path may contain tilde or env vars, so expand them.
    let location = shellexpand::full(location).unwrap().to_string();
    let dir = Path::new(&location);

    // Check if the location exists.
    if !dir.exists() {
        panic!("Location {:?} doesn't exists", dir);
    }

    // If it does, see if it is a dir.
    if !dir.is_dir() {
        panic!("Location {:?} is not a directory", dir);
    }

    // Generate a date.
    //    Format for the date is: 2023-10-Oct-05-19-24-08
    //    date '+%Y-%m-%b-%d-%H-%M-%S'
    let now = Utc::now();
    let filename = now.format("%Y-%m-%b-%d-%H-%M-%S");

    // Create a file in the location with the date as the name.
    let location = dir.join(format!("{}.sh", filename));

    std::fs::File::create(&location).expect("file creation failed");

    // Open vscode in that dump location.
    let _ = Command::new("code")
        .arg(dir)
        .output()
        .expect("failed to execute process");

    // Open that file in vscode.
    let _ = Command::new("code")
        .arg(location)
        .output()
        .expect("failed to execute process");
}
