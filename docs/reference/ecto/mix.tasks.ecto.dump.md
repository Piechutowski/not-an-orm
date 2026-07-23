<!-- Mix.Tasks.Ecto.Dump: extracted verbatim from lib/mix/tasks/ecto.dump.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Dump


Dumps the current environment's database structure for the
given repository into a structure file.

The repository must be set under `:ecto_repos` in the
current app configuration or given via the `-r` option.

This task needs some shell utility to be present on the machine
running the task.

 Database   | Utility needed
 :--------- | :-------------
 PostgreSQL | pg_dump
 MySQL      | mysqldump

## Example

    $ mix ecto.dump

## Command line options

  * `-r`, `--repo` - the repo to load the structure info from
  * `-d`, `--dump-path` - the path of the dump file to create
  * `-q`, `--quiet` - run the command quietly
  * `--no-compile` - does not compile applications before dumping
  * `--no-deps-check` - does not check dependencies before dumping
  * `--prefix` - prefix that will be included in the structure dump.
    Can include multiple prefixes (ex. `--prefix foo --prefix bar`) with
    PostgreSQL but not MySQL. When specified, the prefixes will have
    their definitions dumped along with the data in their migration table.
    The default behavior is dependent on the adapter for backwards compatibility
    reasons. For PostgreSQL, the configured database has the definitions dumped
    from all of its schemas but only the data from the migration table
    from the `public` schema is included. For MySQL, only the configured
    database and its migration table are dumped.
