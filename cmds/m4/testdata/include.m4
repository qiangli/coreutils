dnl include and sinclude resolve names relative to the working directory.
include(`testdata/include-fragment.inc')dnl
from_fragment(`included value')
sinclude(`testdata/does-not-exist.inc')dnl
after silent include
