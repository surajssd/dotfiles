use clap::{arg, command, Command};

mod dump;
use crate::dump::DUMP_LOCATION;

/*
Potential Ideas:

ssd date
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
        .get_matches();

    match matches.subcommand() {
        Some(("dump", sub_matches)) => {
            dump::dump(sub_matches.get_one::<String>("location").unwrap());
        }
        _ => unreachable!("Exhausted list of subcommands"),
    }
}
