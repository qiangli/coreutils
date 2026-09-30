dnl undefine, pushdef, popdef, defn, ifdef.
define(`v', `one')dnl
v pushdef(`v', `two')v pushdef(`v', `three')v
define(`v', `THREE')v popdef(`v')v popdef(`v')v popdef(`v')v popdef(`v')v
pushdef(`w', `1')pushdef(`w', `2')undefine(`w')w
ifdef(`v', `v defined', `v undefined') define(`v')ifdef(`v', `v defined', `v undefined')
ifdef(`define', `builtin defined') [ifdef(`nothing', `unreachable')]
ifdef(`v', `ifdef(`nothing', `no', `nested yes')')
define(`orig', `text with $1')dnl
define(`copy', defn(`orig'))undefine(`orig')dnl
copy(`arg') orig(`arg')
define(`x', `X')define(`holder', `x')dnl
holder defn(`holder')
define(`def2', defn(`define'))dnl
def2(`made', `by def2')made
define(`mylen', defn(`len'))undefine(`len')dnl
mylen(`four') len(`four')
pushdef(`incr2', defn(`incr'))incr2(`41') popdef(`incr2')incr2(`41')
undefine(`ifelse')ifelse(a, a, b)
undefine popdef pushdef defn ifdef
