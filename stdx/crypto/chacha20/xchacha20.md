# XChaCha20

* `key` randomly generated `[32]byte`
* `nonce` `[24]byte`. Either random or counter.
* `chaCha20` is the original chacha20 stream cipher, with a 64 bits blockcounter and 64 bits nonce

```
chacha20Key [32]byte := HChaCha20(key, nonce[0:16])
chacha20Nonce [8]byte := nonce[16:24]

xChaCha20 := chaCha20.New(key = chacha20Key, nonce = chacha20Nonce)
```



The key is required to be 256 bits (32 bytes)
The nonce is required to be 192 bits (24 bytes)
The nonce must be unique for one key for all time.

The XChaCha20 stream cipher can encrypt up to 2^80 messages for each (nonce, key) pair with a random nonce.

The XChaCha20 stream cipher can encrypt up to 2^192 messages for each (nonce, key) pair with a counter nonce.

The XChaCha20 stream cipher can encrypt individual messages of up to 2^64 bytes

XChaCha20 uses a 64 bits counter and the the following state:
```
cccccccc  cccccccc  cccccccc  cccccccc
kkkkkkkk  kkkkkkkk  kkkkkkkk  kkkkkkkk
kkkkkkkk  kkkkkkkk  kkkkkkkk  kkkkkkkk
bbbbbbbb  bbbbbbbb  nnnnnnnn  nnnnnnnn
c=constant k=key b=blockcounter n=nonce
```

which is different than [IETF's draft XChaCha20](https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-xchacha-03)
that use a 32 bits counter and the 32 remaining bits are set to "\x00\x00\x00\x00"

```
cccccccc  cccccccc  cccccccc  cccccccc
kkkkkkkk  kkkkkkkk  kkkkkkkk  kkkkkkkk
kkkkkkkk  kkkkkkkk  kkkkkkkk  kkkkkkkk
bbbbbbbb  00000000  nnnnnnnn  nnnnnnnn

c=constant k=key b=blockcounter n=nonce
```


## Limits
