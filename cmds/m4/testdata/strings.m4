dnl len, index, substr, translit.
len() len(`') len(`a') len(`hello, world') len(`  spaced  ') len(len(`twelve chars')0)
index(`gnus, gnats, and armadillos', `nat') index(`gnus, gnats, and armadillos', `dag')
index(`abc', `') index(`', `') index(`', `a') index(`abcabc', `c') index(`abc', `abcd')
substr(`gnus, gnats, and armadillos', `6') substr(`gnus, gnats, and armadillos', `6', `5')
[substr(`abc', `0')] [substr(`abc', `0', `0')] [substr(`abc', `1', `1')] [substr(`abc', `2', `5')]
[substr(`abc', `3')] [substr(`abc', `4')] [substr(`abc', `-1')] [substr(`abc', `1', `-1')]
define(`first', `substr(`$1', `0', `1')')define(`rest', `substr(`$1', `1')')dnl
first(`hello') rest(`hello')
translit(`GNUs not Unix', `A-Z') translit(`GNUs not Unix', `a-z')
translit(`GNUs not Unix', `a-z', `A-Z') translit(`GNUs not Unix', `A-Z', `z-a')
translit(`hello', `el', `ip') translit(`hello', `l') translit(`hello', `', `x')
translit(`abcdef', `abcd', `xy') translit(`abc', `a', `xyz') translit(`abba', `ab', `ba')
translit(`a-b-c', `-', `_') translit(`path/to/file', `/', `.')
translit(`hello world', `abcdefghijklmnopqrstuvwxyz', `ABCDEFGHIJKLMNOPQRSTUVWXYZ')
define(`upcase', `translit(`$1', `a-z', `A-Z')')dnl
upcase(`shout') upcase(substr(`whisper', `0', `3'))
define(`capitalize', `upcase(first(`$1'))`'rest(`$1')')dnl
capitalize(`word')
len index substr translit
