<!-- Mix.Tasks.Ecto.Migrations: extracted verbatim from lib/mix/tasks/ecto.migrations.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Migrations


Displays the up / down migration status for the given repository.

The repository must be set under `:ecto_repos` in the
current app configuration or given via the `-r` option.

By default, migrations are expected at "priv/YOUR_REPO/migrations"
directory of the current application but it can be configured
by specifying the `:priv` key under the repository configuration.

If the repository has not been started yet, one will be
started outside our application supervision tree and shutdown
afterwards.

## Examples

    $ mix ecto.migrations
    $ mix ecto.migrations -r Custom.Repo

## Command line options

  * `--migrations-path` - the path to load the migrations from, defaults to
    `"priv/repo/migrations"`. This option may be given multiple times in which
    case the migrations are loaded from all the given directories and sorted as
    if they were in the same one.

    Note, if you have previously run migrations from paths `a/` and `b/`, and now
    run `mix ecto.migrations --migrations-path a/` (omitting path `b/`), the
    migrations from the path `b/` will be shown in the output as `** FILE NOT FOUND **`.

  * `--no-compile` - does not compile applications before running

  * `--no-deps-check` - does not check dependencies before running

  * `--prefix` - the prefix to check migrations on

  * `-r`, `--repo` - the repo to obtain the status for
