# Fyne vs Gio - dua aplikasi contoh

Dua aplikasi **identik fungsinya** (Host/Port/Username/Password, satu tombol
Connect/Disconnect yang berubah label, satu baris status), ditulis dengan dua
toolkit Go yang berbeda. Ini contoh untuk membandingkan, **bukan** bagian dari
aplikasi yang dirilis: tidak ada CI rilis yang membangunnya dan modulnya
terpisah dari `desktop/go.mod`, supaya dependency Fyne/Gio tidak masuk ke
produk.

Jalankan (di Linux; di Windows/macOS butuh toolchain cgo-nya sendiri):

```bash
cd samples/gui/fyneapp && go mod tidy && go run .
cd samples/gui/gioapp  && go mod tidy && go run .
```

## Hasil ukur (Linux amd64, `-ldflags "-s -w"`, dijalankan di Xvfb)

| | Fyne | Gio |
|---|---|---|
| Baris kode | 49 | 94 |
| Ukuran binary | **23.901.768 B (22,8 MB)** | **8.864.056 B (8,5 MB)** |
| RAM (RSS, jendela terbuka) | **171,7 MB** (18 thread) | **132,4 MB** (16 thread) |
| Dependency di `go.mod` | ~33 modul | ~8 modul |
| Perlu header build | libgl1-mesa-dev, xorg-dev, libwayland-dev, libxkbcommon-dev | + libegl1-mesa-dev, libx11-xcb-dev, libxkbcommon-x11-dev, libxcursor-dev, libxfixes-dev, libvulkan-dev |
| Cara menyusun UI | widget + container (`widget.NewForm`, `container.NewVBox`) | gambar manual tiap frame (`layout.Flex`, `layout.Rigid`) |
| Font/tema | dibawa sendiri (ikut masuk binary) | font sistem atau gofont; tema material |
| Platform | desktop + Android + iOS + WebAssembly + embedded Linux | desktop + Android + iOS + WebAssembly (eksperimental) + FreeBSD/OpenBSD |

Catatan: RSS di atas adalah **batas atas** - Xvfb tidak punya GPU, jadi keduanya
menggambar lewat GL software (llvmpipe) dan seluruh buffer ada di RAM. Dengan GPU
asli angka ini turun.

Sebagai pembanding, GUI yang benar-benar dirilis repo ini:

| | GTK3 (Linux) | AppKit (macOS) | walk (Windows) |
|---|---|---|---|
| Ukuran binary | 5,9 MB | 5,5 MB | 10,3 MB |
| RAM | 7,0 MB terukur (Xvfb) | belum diukur | ~27 MB |

Itu sebabnya dua shell native di `desktop/cmd/` tetap dipakai: toolkit-nya sudah
dimuat sistem, jadi binary dan RAM-nya jauh lebih kecil daripada membawa mesin
gambar sendiri.
