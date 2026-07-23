<!-- Ecto.Adapters.SQLite3.TypeExtension: extracted verbatim from lib/ecto/adapters/sqlite3/type_extension.ex (ecto_sqlite3 repo) by fetch.sh. MIT. -->

# Ecto.Adapters.SQLite3.TypeExtension


A behaviour that defines the API for extensions providing custom data loaders and dumpers
for Ecto schemas.


---

## `@callback loaders(primitive_type, ecto_type) :: list | nil`

Takes a primitive type and, if it knows how to decode it into an
appropriate Elixir data structure, returns a two-element list in
the form `[(db_data :: any -> term), elixir_type :: atom]`.

The function that is the first element will be called whenever
the `primitive_type` appears in a schema and data is fetched from
the database for it.


---

## `@callback dumpers(ecto_type, primitive_type) :: list | nil`

Takes a primitive type and, if it knows how to encode it for storage
in the database, returns a two-element list in
the form `[elixir_type :: atom, (term -> db_data :: any)]`.

The function that is the second element will be called whenever
the `primitive_type` appears in a schema and data is about to be
sent to the database for storage.
