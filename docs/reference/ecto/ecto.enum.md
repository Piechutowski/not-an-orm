<!-- Ecto.Enum: extracted verbatim from lib/ecto/enum.ex (ecto repo) by fetch.sh. Apache-2.0. -->

# Ecto.Enum


A custom type that maps atoms to strings or integers.

`Ecto.Enum` must be used whenever you want to keep atom values in a field.
Since atoms cannot be persisted to the database, `Ecto.Enum` converts them
to strings or integers when writing to the database and converts them back
to atoms when loading data. It can be used in your schemas as follows:

    # Stored as strings
    field :status, Ecto.Enum, values: [:foo, :bar, :baz]

or

    # Stored as integers
    field :status, Ecto.Enum, values: [foo: 1, bar: 2, baz: 5]

Therefore, the type to be used in your migrations for enum fields depends
on the choice above. For the cases above, one would do, respectively:

    add :status, :string

or

    add :status, :integer

Some databases also support enum types, which you could use in combination
with the above.

Composite types, such as `:array`, are also supported which allow selecting
multiple values per record:

    field :roles, {:array, Ecto.Enum}, values: [:author, :editor, :admin]

Overall, `:values` must be a list of atoms or a keyword list. Values will be
cast to atoms safely and only if the atom exists in the list (otherwise an
error will be raised). Attempting to load any string/integer not represented
by an atom in the list will be invalid.

The helper function `mappings/2` returns the mappings for a given schema and
field, which can be used in places like form drop-downs. See `mappings/2` for
examples.

If you want the values only, you can use `values/2`, and if you want
the "dump-able" values only, you can use `dump_values/2`.

## Embeds

`Ecto.Enum` allows to customize how fields are dumped within embeds through the
`:embed_as` option. Two alternatives are supported: `:values`, which will save
the enum keys (and not their respective mapping), and `:dumped`, which will save
the dumped value. The default is `:values`. For example, assuming the following
schema:

    defmodule EnumSchema do
      use Ecto.Schema

      schema "my_schema" do
        embeds_one :embed, Embed do
          field :embed_as_values, Ecto.Enum, values: [foo: 1, bar: 2], embed_as: :values
          field :embed_as_dump, Ecto.Enum, values: [foo: 1, bar: 2], embed_as: :dumped
        end
      end
    end

The `:embed_as_values` field value will save `:foo` or `:bar`, while the
`:embed_as_dump` field value will save `1` or `2`.


---

## `values(schema_or_types, field)`

Returns the possible values for a given schema or types map and field.

These values are the atoms that represent the different possible values
of the field.

## Examples

Assuming this schema:

    defmodule MySchema do
      use Ecto.Schema

      schema "my_schema" do
        field :my_string_enum, Ecto.Enum, values: [:foo, :bar, :baz]
        field :my_integer_enum, Ecto.Enum, values: [foo: 1, bar: 2, baz: 5]
      end
    end

Then:

    Ecto.Enum.values(MySchema, :my_string_enum)
    #=> [:foo, :bar, :baz]

    Ecto.Enum.values(MySchema, :my_integer_enum)
    #=> [:foo, :bar, :baz]


---

## `dump_values(schema_or_types, field)`

Returns the possible dump values for a given schema or types map and field

"Dump values" are the values that can be dumped in the database. For enums stored
as strings, these are the strings that will be dumped in the database. For enums
stored as integers, these are the integers that will be dumped in the database.

## Examples

Assuming this schema:

    defmodule MySchema do
      use Ecto.Schema

      schema "my_schema" do
        field :my_string_enum, Ecto.Enum, values: [:foo, :bar, :baz]
        field :my_integer_enum, Ecto.Enum, values: [foo: 1, bar: 2, baz: 5]
      end
    end

Then:

    Ecto.Enum.dump_values(MySchema, :my_string_enum)
    #=> ["foo", "bar", "baz"]

    Ecto.Enum.dump_values(MySchema, :my_integer_enum)
    #=> [1, 2, 5]

`schema_or_types` can also be a types map. See `mappings/2` for more information.


---

## `cast_value(schema_or_types, field, value)`

Casts a value from the given `schema` and `field`.

## Examples

Assuming this schema:

    defmodule MySchema do
      use Ecto.Schema

      schema "my_schema" do
        field :my_string_enum, Ecto.Enum, values: [:foo, :bar, :baz]
        field :my_integer_enum, Ecto.Enum, values: [foo: 1, bar: 2, baz: 5]
      end
    end

Then:

    Ecto.Enum.cast_value(MySchema, :my_string_enum, "foo")
    #=> {:ok, :foo}

    Ecto.Enum.cast_value(MySchema, :my_string_enum, :foo)
    #=> {:ok, :foo}

    Ecto.Enum.cast_value(MySchema, :my_string_enum, "qux")
    #=> :error

    Ecto.Enum.cast_value(MySchema, :my_integer_enum, 1)
    #=> {:ok, :foo}

    Ecto.Enum.cast_value(MySchema, :my_integer_enum, :foo)
    #=> {:ok, :foo}

    Ecto.Enum.cast_value(MySchema, :my_integer_enum, 6)
    #=> :error

`schema_or_types` can also be a types map. See `mappings/2` for more information.


---

## `mappings(schema_or_types, field)`

Returns the mappings between values and dumped values.

## Examples

Assuming this schema:

    defmodule MySchema do
      use Ecto.Schema

      schema "my_schema" do
        field :my_string_enum, Ecto.Enum, values: [:foo, :bar, :baz]
        field :my_integer_enum, Ecto.Enum, values: [foo: 1, bar: 2, baz: 5]
      end
    end

Here are some examples of using `mappings/2` with it:

    Ecto.Enum.mappings(MySchema, :my_string_enum)
    #=> [foo: "foo", bar: "bar", baz: "baz"]

    Ecto.Enum.mappings(MySchema, :my_integer_enum)
    #=> [foo: 1, bar: 2, baz: 5]

Examples of calling `mappings/2` with a types map:

    schemaless_types = %{
      my_enum: Ecto.ParameterizedType.init(Ecto.Enum, values: [:foo, :bar, :baz]),
      my_integer_enum: Ecto.ParameterizedType.init(Ecto.Enum, values: [foo: 1, bar: 2, baz: 5])
    }

    Ecto.Enum.mappings(schemaless_types, :my_enum)
    #=> [foo: "foo", bar: "bar", baz: "baz"]
    Ecto.Enum.mappings(schemaless_types, :my_integer_enum)
    #=> [foo: 1, bar: 2, baz: 5]
