// OCR helper for macOS: prints the text of an image or PDF using Apple's
// Vision framework, one line per text line, pages separated by a form feed.
// Usage: ocr-vision <file> <pdf|image>
import AppKit
import Foundation
import PDFKit
import Vision

func images(_ path: String, _ kind: String) -> [CGImage] {
    let url = URL(fileURLWithPath: path)
    if kind == "pdf" {
        guard let doc = PDFDocument(url: url) else { return [] }
        var out: [CGImage] = []
        for i in 0..<min(doc.pageCount, 10) {
            guard let page = doc.page(at: i) else { continue }
            let box = page.bounds(for: .mediaBox)
            let scale = 300.0 / 72.0 // render at 300 dpi
            let size = NSSize(width: box.width * scale, height: box.height * scale)
            let img = page.thumbnail(of: size, for: .mediaBox)
            var rect = NSRect(origin: .zero, size: img.size)
            if let cg = img.cgImage(forProposedRect: &rect, context: nil, hints: nil) { out.append(cg) }
        }
        return out
    }
    guard let src = CGImageSourceCreateWithURL(url as CFURL, nil),
          let cg = CGImageSourceCreateImageAtIndex(src, 0, nil) else { return [] }
    return [cg]
}

func orientation(_ path: String, _ kind: String) -> CGImagePropertyOrientation {
    guard kind != "pdf",
          let src = CGImageSourceCreateWithURL(URL(fileURLWithPath: path) as CFURL, nil),
          let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any],
          let raw = props[kCGImagePropertyOrientation] as? UInt32,
          let o = CGImagePropertyOrientation(rawValue: raw) else { return .up }
    return o
}

let args = CommandLine.arguments
guard args.count == 3 else {
    FileHandle.standardError.write("usage: ocr-vision <file> <pdf|image>\n".data(using: .utf8)!)
    exit(2)
}
let pages = images(args[1], args[2])
if pages.isEmpty {
    FileHandle.standardError.write("could not read the file\n".data(using: .utf8)!)
    exit(1)
}
let wanted = ["en-US", "ru-RU", "uk-UA", "it-IT", "de-DE", "fr-FR"]
var out: [String] = []
for page in pages {
    let req = VNRecognizeTextRequest()
    req.recognitionLevel = .accurate
    req.usesLanguageCorrection = false // keeps machine-readable zones and numbers literal
    if let supported = try? req.supportedRecognitionLanguages() {
        req.recognitionLanguages = wanted.filter { supported.contains($0) }
    }
    let handler = VNImageRequestHandler(cgImage: page, orientation: orientation(args[1], args[2]), options: [:])
    do { try handler.perform([req]) } catch {
        FileHandle.standardError.write("ocr failed: \(error)\n".data(using: .utf8)!)
        exit(1)
    }
    // Reading order: top to bottom, then left to right.
    let obs = (req.results ?? []).sorted {
        abs($0.boundingBox.midY - $1.boundingBox.midY) > 0.01 ? $0.boundingBox.midY > $1.boundingBox.midY : $0.boundingBox.minX < $1.boundingBox.minX
    }
    out.append(obs.compactMap { $0.topCandidates(1).first?.string }.joined(separator: "\n"))
}
print(out.joined(separator: "\n\u{0C}\n"))
