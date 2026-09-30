dnl Comments are copied, never expanded.
define(`x', `expanded')dnl
x # x `x' define(`y', `oops')
x y
define(`show', `[$1]')dnl
show(x # x, still the first argument )
, second)
changecom(`/*', `*/')dnl
x /* x
 x */ x # x
show(/* a, b) */ x)
changecom(`//')dnl
x // x
x /* x */ # x
changecom dnl
x # x // x /* x */
changecom(`#')dnl
x # x
changecom(`<!--', `-->')dnl
x <!-- x --> x <!- x -> # x
