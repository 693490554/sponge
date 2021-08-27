package common

func KTIsOk(kt KT) bool {
	return kt == KTOfHash || kt == KTOfString
}
