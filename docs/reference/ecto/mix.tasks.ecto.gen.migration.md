<!-- Mix.Tasks.Ecto.Gen.Migration: extracted verbatim from lib/mix/tasks/ecto.gen.migration.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Gen.Migration


Generates a migration.

The repository must be set under `:ecto_repos` in the
current app configuration or given via the `-r` option.

## Examples

    $ mix ecto.gen.migration add_posts_table
    $ mix ecto.gen.migration add_posts_table -r Custom.Repo

The generated migration filename will be prefixed with the current
timestamp in UTC which is used for versioning and ordering.

By default, the migration will be generated to the
"priv/YOUR_REPO/migrations" directory of the current application
but it can be configured to be any subdirectory of `priv` by
specifying the `:priv` key under the repository configuration.

This generator will automatically open the generated file if
you have `ECTO_EDITOR` set in your environment variable.

## Command line options

  * `-r`, `--repo` - the repo to generate migration for
  * `--no-compile` - does not compile applications before running
  * `--no-deps-check` - does not check dependencies before running
  * `--migrations-path` - the path to run the migrations from, defaults to `priv/repo/migrations`

## Configuration

If the current app configuration specifies a custom migration module
the generated migration code will use that rather than the default
`Ecto.Migration`:

    config :ecto_sql, migration_module: MyApplication.CustomMigrationModule
