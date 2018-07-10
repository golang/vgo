module m

require (
	a v1.0.0
	mysite/myname/mypkg v1.0.0
	w v1.0.0 // indirect
	x v1.0.0
	y v1.0.0
	z v1.0.0
)

replace (
	a v1.0.0 => ./a
	mysite/myname/mypkg v1.0.0 => ./mypkg
	w v1.0.0 => ./w
	x v1.0.0 => ./x
	y v1.0.0 => ./y
	z v1.0.0 => ./z
)
