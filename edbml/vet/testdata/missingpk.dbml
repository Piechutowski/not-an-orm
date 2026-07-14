// analyzers: missingpk
Table has_col_pk {
  id int [pk]
}
Table has_index_pk {
  a int
  b int
  indexes {
    (a, b) [pk]
  }
}
Table has_legacy_pk {
  id int pk
}
Table naked { //WANT missingpk
  id int
}
