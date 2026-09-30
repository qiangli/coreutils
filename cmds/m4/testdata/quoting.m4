dnl Quoting, rescanning and argument collection.
define(`a', `b')define(`b', `c')define(`c', `done')dnl
a `a' ``a'' ```a'''
define(`lit', ``a'')lit
define(`pair', `<$1|$2>')dnl
pair(a, b) pair(`a', `b') pair(``a'', ``b'')
pair(  leading,   spaces  ) pair(
  on,
  lines)
pair((x, y), z) pair(`x, y', z) pair((((deep, er))), (z))
define(`list', `1, 2')dnl
pair(list) pair(`list') pair((list))
pair(pair(1, 2), pair(3, 4))
pair (not, a call)
a`'a a`'b`'c xa ax a_ _a a1 1a
define(`cat', `$1$2$3')dnl
cat(`un', `de', `fined') cat(a, `a', ``a'')
`unbalanced ) paren', `and a , comma'
pair(`)', `(') pair(`,', `,')
changequote([, ])dnl
define([q], [[$1] in brackets])dnl
q(a) [a] [[a]] `a'
changequote([<<], [>>])dnl
define(<<r>>, <<<<$1>> in angles>>)dnl
r(a) <<a>> <<<<a>>>> [a] < a > <a>
changequote`'dnl
`a' [a] <<a>> a
changequote(`"', `"')dnl
"a" ""a "a b" a
changequote(`{')dnl
{a' {{a'' a
changequote
