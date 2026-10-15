//go:build amd64 && gc && !purego

#include "const.s"
#include "macro.s"

#define Dst DI
#define Nonce AX
#define Key BX
#define Rounds DX

// func hChaCha20AVX(out *[32]byte, nonce *[16]byte, key *[32]byte)
TEXT ·hChaCha20AVX(SB), 4, $0-24
	MOVQ out+0(FP), Dst
	MOVQ nonce+8(FP), Nonce
	MOVQ key+16(FP), Key

	VMOVDQU ·sigma<>(SB), X0
	VMOVDQU 0*16(Key), X1
	VMOVDQU 1*16(Key), X2
	VMOVDQU 0*16(Nonce), X3
	VMOVDQU ·rol16_AVX2<>(SB), X5
	VMOVDQU ·rol8_AVX2<>(SB), X6
	MOVQ    $20, Rounds

CHACHA_LOOP:
	CHACHA_QROUND_AVX(X0, X1, X2, X3, X4, X5, X6)
	CHACHA_SHUFFLE_AVX(X1, X2, X3)
	CHACHA_QROUND_AVX(X0, X1, X2, X3, X4, X5, X6)
	CHACHA_SHUFFLE_AVX(X3, X2, X1)
	SUBQ $2, Rounds
	JNZ  CHACHA_LOOP

	VMOVDQU X0, 0*16(Dst)
	VMOVDQU X3, 1*16(Dst)
	VZEROUPPER
	RET

// func hChaCha20SSE2(out *[32]byte, nonce *[16]byte, key *[32]byte)
TEXT ·hChaCha20SSE2(SB), 4, $0-24
	MOVQ out+0(FP), Dst
	MOVQ nonce+8(FP), Nonce
	MOVQ key+16(FP), Key

	MOVOU ·sigma<>(SB), X0
	MOVOU 0*16(Key), X1
	MOVOU 1*16(Key), X2
	MOVOU 0*16(Nonce), X3
	MOVQ  $20, Rounds

CHACHA_LOOP:
	CHACHA_QROUND_SSE2(X0, X1, X2, X3, X4)
	CHACHA_SHUFFLE_SSE(X1, X2, X3)
	CHACHA_QROUND_SSE2(X0, X1, X2, X3, X4)
	CHACHA_SHUFFLE_SSE(X3, X2, X1)
	SUBQ $2, Rounds
	JNZ  CHACHA_LOOP

	MOVOU X0, 0*16(Dst)
	MOVOU X3, 1*16(Dst)
	RET

// func hChaCha20SSSE3(out *[32]byte, nonce *[16]byte, key *[32]byte)
TEXT ·hChaCha20SSSE3(SB), 4, $0-24
	MOVQ out+0(FP), Dst
	MOVQ nonce+8(FP), Nonce
	MOVQ key+16(FP), Key

	MOVOU ·sigma<>(SB), X0
	MOVOU 0*16(Key), X1
	MOVOU 1*16(Key), X2
	MOVOU 0*16(Nonce), X3
	MOVOU ·rol16<>(SB), X5
	MOVOU ·rol8<>(SB), X6
	MOVQ  $20, Rounds

chacha_loop:
	CHACHA_QROUND_SSSE3(X0, X1, X2, X3, X4, X5, X6)
	CHACHA_SHUFFLE_SSE(X1, X2, X3)
	CHACHA_QROUND_SSSE3(X0, X1, X2, X3, X4, X5, X6)
	CHACHA_SHUFFLE_SSE(X3, X2, X1)
	SUBQ $2, Rounds
	JNZ  chacha_loop

	MOVOU X0, 0*16(Dst)
	MOVOU X3, 1*16(Dst)
	RET

#undef Dst
#undef Nonce
#undef Key
#undef Rounds
