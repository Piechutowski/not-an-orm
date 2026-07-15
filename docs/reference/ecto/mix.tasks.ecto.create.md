<!-- Mix.Tasks.Ecto.Create: extracted verbatim from lib/mix/tasks/ecto.create.ex (ecto repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Create


Create the storage for the given repository.

The repositories to create are the ones specified under the
`:ecto_repos` option in the current app configuration. However,
if the `-r` option is given, it replaces the `:ecto_repos` config.

Since Ecto tasks can only be executed once, if you need to create
multiple repositories, set `:ecto_repos` accordingly or pass the `-r`
flag multiple times.

## Examples

    $ mix ecto.create
    $ mix ecto.create -r Custom.Repo

## Command line options

  * `-r`, `--repo` - the repo to create
  * `--quiet` - do not log output
  * `--no-compile` - do not compile before creating
  * `--no-deps-check` - do not compile before creating
  * `--timezone` - the timezone of the database. By default,
    it is Etc/UTC. Of the default database adapters, only
    PostgreSQL supports timezone. Use `--no-timezone` to use
    the default timezone of the database
