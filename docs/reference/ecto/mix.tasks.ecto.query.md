<!-- Mix.Tasks.Ecto.Query: extracted verbatim from lib/mix/tasks/ecto.query.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Mix.Tasks.Ecto.Query


Runs the given query against the repository.

The query is evaluated as Elixir code after importing `Ecto.Query`.
If a local `.iex.exs` file exists, only aliases from the file are made
available to the query.

The query runs inside a read-only transaction.

## Examples

    $ mix ecto.query "from p in Post, where: p.published"
    $ mix ecto.query -r Custom.Repo "from p in Post, limit: 10"
    $ mix ecto.query --sql "from p in Post, where: p.published"

## Command line options

  * `-r`, `--repo` - the repo to query
  * `--limit` - limits the number of printed entries. Defaults to 100.
  * `--sql` - prints the generated SQL and parameters instead of running the query
