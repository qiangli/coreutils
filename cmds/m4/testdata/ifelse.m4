dnl ifelse, shift and bounded recursion.
ifelse(`a', `a', `same', `different') ifelse(`a', `b', `same', `different')
[ifelse(`a', `b', `same')] [ifelse(`comment only')] [ifelse(`a', `a', `')]
ifelse(`1', `2', `first', `3', `3', `second', `default')
ifelse(`1', `2', `first', `3', `4', `second', `default')
[ifelse(`1', `2', `first', `3', `4', `second')]
ifelse(`1', `2', `a', `3', `4', `b', `5', `5', `c', `d')
define(`x', `1')dnl
ifelse(x, `1', `x is one', `x is not one') ifelse(`x', `1', `quoted x is one', `quoted x is not one')
ifelse(, , `empty equals empty') ifelse( a, a, `leading space stripped')
ifelse(len(`abc'), `3', `length three')
shift(`a', `b', `c') [shift(`a')] [shift()]
define(`argn', `$#')dnl
argn(shift(`a', `b, c', `d')) argn(shift(`a'))
define(`countdown', `$1`'ifelse($1, `0', `', ` countdown(decr($1))')')dnl
countdown(`10')
define(`reverse', `ifelse($#, `0', `', $#, `1', ``$1'', `reverse(shift($@)), `$1'')')dnl
reverse(`a', `b', `c', `d') reverse(`solo') [reverse]
define(`join', `ifelse($#, `2', ``$2'', `ifelse(`$2', `', `', ``$2'`$1'')join(`$1', shift(shift($@)))')')dnl
join(`-', `a', `b', `c') join(`, ', `x', `y')
define(`for', `ifelse(eval($2 <= $3), `1', `pushdef(`$1', `$2')$4`'popdef(`$1')for(`$1', incr($2), `$3', `$4')')')dnl
for(`i', `1', `5', `i*i=eval(i * i) ')
define(`fact', `ifelse($1, `0', `1', `eval($1 * fact(decr($1)))')')dnl
fact(`0') fact(`1') fact(`5') fact(`10')
define(`fib', `ifelse(eval($1 < 2), `1', `$1', `eval(fib(decr($1)) + fib(decr(decr($1))))')')dnl
fib(`0') fib(`1') fib(`2') fib(`10')
