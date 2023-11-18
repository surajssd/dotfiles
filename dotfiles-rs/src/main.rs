use clap::{arg, command, Command};

mod dump;
use crate::dump::DUMP_LOCATION;
mod chrome_pastable;
mod date;

/*
Potential Ideas:

ssd git po
ssd git pum
*/

fn main() {
    let matches = command!()
        .propagate_version(true)
        .arg_required_else_help(true)
        .subcommand(
            Command::new("dump")
                .about(format!("Create a dump file in default location: {}", dump::DUMP_LOCATION))
                .arg(arg!(-l --location <LOCATION> "Optionally specify the location to use as a dump location").default_value(DUMP_LOCATION)),
        )
        .subcommand(
            Command::new("date")
            .about("Print the date right now in the format: %Y-%m-%b-%d-%H-%M-%S. e.g. 2023-10-Oct-05-19-24-08")
        )
        .subcommand(
            Command::new("chrome-pastable")
            .about("Make chrome pastable")
        )
        .get_matches();

    match matches.subcommand() {
        Some(("dump", sub_matches)) => {
            dump::dump(sub_matches.get_one::<String>("location").unwrap());
        }
        Some(("date", _)) => {
            println!("{}", date::date());
        }
        Some(("chrome-pastable", _)) => {
            chrome_pastable::chrome();
        }
        _ => unreachable!("Exhausted list of subcommands"),
    }
}
