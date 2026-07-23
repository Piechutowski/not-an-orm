<!-- Mix.Tasks.Ecto.Drop: extracted verbatim from lib/mix/tasks/ecto.drop.ex (ecto repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Drop


Drop the storage for the given repository.

The repositories to drop are the ones specified under the
`:ecto_repos` option in the current app configuration. However,
if the `-r` option is given, it replaces the `:ecto_repos` config.

Since Ecto tasks can only be executed once, if you need to drop
multiple repositories, set `:ecto_repos` accordingly or pass the `-r`
flag multiple times.

## Examples

    $ mix ecto.drop
    $ mix ecto.drop -r Custom.Repo

## Command line options

  * `-r`, `--repo` - the repo to drop
  * `-q`, `--quiet` - run the command quietly
  * `-f`, `--force` - do not ask for confirmation when dropping the database.
    Configuration is asked only when `:start_permanent` is set to true
    (typically in production)
  * `--force-drop` - force the database to be dropped even
    if it has connections to it (requires PostgreSQL 13+)
  * `--no-compile` - do not compile before dropping
  * `--no-deps-check` - do not compile before dropping
