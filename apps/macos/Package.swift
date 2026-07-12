// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "DevHearth",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "DevHearth", targets: ["DevHearthApp"])],
    targets: [
        .executableTarget(name: "DevHearthApp"),
        .testTarget(name: "DevHearthAppTests", dependencies: ["DevHearthApp"]),
    ]
)
