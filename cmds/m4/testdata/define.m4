dnl Macro definition and parameter substitution.
define(`greeting', `Hello, $1!')dnl
greeting(`world')
greeting(`m4', `ignored')
greeting
define(`count', `$#')dnl
count count() count(a) count(a, b) count(a, b, c) count((a, b), c)
define(`all', `[$*]')define(`each', `[$@]')dnl
define(`x', `X')dnl
all(`x', `y', `z') each(`x', `y', `z')
define(`swap', `$2 $1')dnl
swap(`one', `two')
define(`nine', `$9$8$7$6$5$4$3$2$1')dnl
nine(a, b, c, d, e, f, g, h, i)
nine(a, b, c)
define(`self', ``$0' was called with $# argument(s)')dnl
self self(1, 2)
define(`money', `$ $x $1$ $$1')dnl
money(`five')
define(`empty')dnl
[empty] [empty(1, 2)]
define(`redef', `first')redef define(`redef', `second')redef
define(`outer', `define(`inner', `got $1 and $'`1')')dnl
outer(`A')inner(`B')
define bare, and define (not a call)
