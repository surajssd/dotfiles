use chrono::Utc;

pub fn date() -> String {
    let now = Utc::now();

    // Generate a date:
    //    Format for the date is: 2023-10-Oct-05-19-24-08
    //    date '+%Y-%m-%b-%d-%H-%M-%S'
    now.format("%Y-%m-%b-%d-%H-%M-%S").to_string()
}
