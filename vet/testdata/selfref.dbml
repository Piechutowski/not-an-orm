// analyzers: selfref
Table employees {
  id int [pk]
  manager_id int
}
Ref: employees.manager_id > employees.id
Ref weird: employees.id - employees.id //WANT selfref
