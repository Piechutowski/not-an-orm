<!-- Mix.Tasks.Ecto.Load: extracted verbatim from lib/mix/tasks/ecto.load.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Load


Loads the current environment's database structure for the
given repository from a previously dumped structure file.

The repository must be set under `:ecto_repos` in the
current app configuration or given via the `-r` option.

This task needs some shell utility to be present on the machine
running the task.

 Database   | Utility needed
 :--------- | :-------------
 PostgreSQL | psql
 MySQL      | mysql

## Example

    $ mix ecto.load

## Command line options

  * `-r`, `--repo` - the repo to load the structure info into
  * `-d`, `--dump-path` - the path of the dump file to load from
  * `-q`, `--quiet` - run the command quietly
  * `-f`, `--force` - do not ask for confirmation when loading data.
    Configuration is asked only when `:start_permanent` is set to true
    (typically in production)
  * `--no-compile` - does not compile applications before loading
  * `--no-deps-check` - does not check dependencies before loading
  * `--skip-if-loaded` - does not load the dump file if the repo has the migrations table up
