dnl incr, decr and eval.
incr(`0') incr(`41') incr(`-1') incr(`-100') decr(`0') decr(`1') decr(`-5') decr(`100')
incr(incr(incr(`0'))) decr(incr(`7')) incr(2147483647) decr(-2147483648)
eval(`1 + 2') eval(`1 + 2 * 3') eval(`(1 + 2) * 3') eval(`10 - 2 - 3') eval(`100 / 10 / 5')
eval(`7 / 2') eval(`-7 / 2') eval(`7 / -2') eval(`7 % 3') eval(`-7 % 3') eval(`7 % -3')
eval(`-3') eval(`+3') eval(`-(-3)') eval(`-(2 + 3)') eval(`!0') eval(`!7') eval(`!!7') eval(`~0') eval(`~5') eval(`-(~5)')
eval(`1 < 2') eval(`2 < 1') eval(`2 <= 2') eval(`3 <= 2') eval(`3 > 2') eval(`2 > 3') eval(`2 >= 2') eval(`1 >= 2')
eval(`2 == 2') eval(`2 == 3') eval(`2 != 2') eval(`2 != 3') eval(`1 < 2 == 1') eval(`3 > 2 > 1')
eval(`6 & 3') eval(`6 | 3') eval(`6 ^ 3') eval(`1 | 2 ^ 3 & 4') eval(`12 & 10 | 1') eval(`5 ^ 5')
eval(`1 << 4') eval(`1 << 31') eval(`256 >> 4') eval(`-16 >> 2') eval(`1 + 2 << 3') eval(`1 << 2 + 3')
eval(`2 && 3') eval(`2 && 0') eval(`0 && 0') eval(`0 || 3') eval(`0 || 0') eval(`1 || 0 && 0') eval(`!1 || !0')
eval(`1 + 1 == 2 && 2 * 2 == 4') eval(`1 & 2 == 2') eval(`(1 & 2) == 2') eval(`4 | 1 && 0')
eval(`010') eval(`0x1f') eval(`0X1F') eval(`0xff + 010 + 9') eval(`0') eval(`00') eval(`0x0')
eval(`2147483647') eval(`2147483647 + 1') eval(`-2147483647 - 1') eval(`65536 * 65536') eval(`46341 * 46341')
eval(` 1+1 ') eval(`1	+
1') eval(`((((5))))') eval(`2*(3+(4-1))/3')
eval(`255', `16') eval(`255', `2') eval(`255', `8') eval(`255', `10') eval(`255', `36') eval(`35', `36')
eval(`-255', `16') eval(`0', `2') eval(`5', `1') eval(`0', `1') eval(`-3', `1')
eval(`5', `10', `3') eval(`-5', `10', `3') eval(`255', `16', `8') eval(`-5', `2', `8') eval(`12345', `10', `2') eval(`0', `10', `0') eval(`7', `', `4')
define(`n', `6')define(`sq', `eval($1 * $1)')dnl
eval(n * 7) sq(`12') sq(sq(`3')) eval(sq(n) + incr(n))
eval incr decr
