# Virus Uyarıları Hakkında Bilgilendirme

OpenFliqlo, Go dili ile geliştirilmiş açık kaynaklı ve güvenli bir yazılımdır. Ancak, Go ile yazılan uygulamalar (özellikle ekranda tam ekran çalışan .scr dosyaları) bazen antivirüs yazılımları tarafından "tehdit" olarak işaretlenebilir.

## Neden Uyarı Alıyorum?

1.  **Dijital İmza Eksikliği:** Uygulama ücretli bir dijital sertifika ile imzalanmamıştır. Windows, imzalanmamış dosyaları şüpheli görebilir.
2.  **Go'nun Çalışma Yapısı:** Go dili, kodları tek bir büyük dosya (static binary) haline getirir. Bazı antivirüslerin sezgisel tarayıcıları bu yapıyı yanlış anlayabilir.
3.  **Heuristic (Sezgisel) Algılama:** Ekran koruyucular sistem üzerinde tam yetkiyle çalıştığı için (fare ve klavye hareketlerini dinlemek gibi), antivirüsler bunu "tehlikeli bir klavye dinleyici" sanabilir.

## Uyarıyı Nasıl Gideririm?

Biz bu uyarıları azaltmak için uygulamaya **Metadata (Kimlik Bilgileri)** ve **Windows Manifest** ekledik. Eğer hala uyarı alıyorsanız şu adımları izleyebilirsiniz:

### 1. Dosyayı Taratın ve İstisna Ekleyin
Dosyayı VirusTotal gibi sitelerde taratabilir ve sonucun temiz olduğunu (genelde 0/70 veya 1/70 hatalı alarm) görebilirsiniz. Kendi bilgisayarınızda dosyaya güveniyorsanız, Windows Defender'da "İstisna" (Exclusion) olarak ekleyebilirsiniz.

### 2. Microsoft'a "Güvenli" Olarak Bildirin
Microsoft SmartScreen uyarısı veriyorsa, dosyayı Microsoft'un analiz portalına yükleyerek "Hatayı Bildir" diyebilirsiniz. Genelde 24-48 saat içinde dosyayı beyaz listeye alırlar.

Microsoft Analiz Portalı: [https://www.microsoft.com/en-us/wdsi/filesubmission](https://www.microsoft.com/en-us/wdsi/filesubmission)

---
*Proje açık kaynak kodludur. Dilerseniz tüm kodu `main.go` dosyasından inceleyebilir ve kendiniz derleyebilirsiniz.*
