dnl A small document generator exercising the builtins together.
define(`_counter', `0')dnl
define(`next', `define(`_counter', incr(_counter))_counter')dnl
define(`section', `next. upcase(`$1')
underline(len(`$1'), `=')')dnl
define(`upcase', `translit(`$1', `a-z', `A-Z')')dnl
define(`underline', `ifelse($1, `0', `', `$2`'underline(decr($1), `$2')')')dnl
define(`item', `  * $1`'ifelse(`$2', `', `', ` ($2)')')dnl
section(`introduction')
item(`first point')
item(`second point', `with a note')
section(`details')
item(`count so far: _counter')
define(`table', `ifelse($#, `0', `', $#, `1', `', `$1 = $2
table(shift(shift($@)))')')dnl
table(`alpha', `1', `beta', `2', `gamma', `3')dnl
pushdef(`upcase', `<$1>')section(`overridden')
popdef(`upcase')section(`restored')
ifdef(`table', ``table' is defined', ``table' is missing'), ifdef(`chair', `chair is defined', `chair is missing')
changequote([, ])dnl
define([bracket], [[$1] has len([$1]) characters])dnl
bracket([some, text])
changequote`'dnl
substr(`Hello, World', index(`Hello, World', `W'))
# a comment: section(`not expanded') _counter
_counter sections
