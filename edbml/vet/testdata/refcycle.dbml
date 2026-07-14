// analyzers: refcycle
Table a {
  id int [pk]
  b_id int
}
Table b { //WANT refcycle
  id int [pk]
  a_id int
}
Table c {
  id int [pk]
  a_id int [ref: > a.id]
}
Ref: a.b_id > b.id
Ref: b.a_id > a.id
