# frozen_string_literal: true

require_relative "package_sources"
require "minitest/autorun"

module PackageTestHelpers
  def in_temporary_directory
    Dir.mktmpdir("hyprmoncfg-test-") { |path| yield Pathname.new(path) }
  end

  def snapshot(directory)
    Packages.files(directory).to_h { |path| [path.relative_path_from(directory).to_s, path.binread] }
  end

  def metadata
    result = {
      "VERSION" => "9.8.7", "COMMIT" => "abcdef0", "FULL_COMMIT" => "abcdef0" * 5 + "abcde",
      "GO_VERSION" => "1.26.1", "GO_MINOR" => "1.26",
      "RPM_DATE" => "Sat Sep 12 2026", "DEBIAN_DATE" => "Sat, 12 Sep 2026 12:00:00 +0000",
      "MAN_DATE" => "September 2026", "NIX_VENDOR_SRI" => "sha256-#{'A' * 43}="
    }
    names = {
      "SOURCE" => "hyprmoncfg-9.8.7.tar.gz", "DEPS" => "hyprmoncfg-9.8.7-deps.tar.xz",
      "AMD64" => "hyprmoncfg_9.8.7_linux_amd64.tar.gz", "ARM64" => "hyprmoncfg_9.8.7_linux_arm64.tar.gz"
    }
    names.each do |kind, name|
      result.merge!(
        "#{kind}_NAME" => name, "#{kind}_URL" => "https://example.org/#{name}",
        "#{kind}_SIZE" => 123, "#{kind}_SHA256" => "a" * 64, "#{kind}_SHA512" => "b" * 128,
        "#{kind}_MD5" => "c" * 32, "#{kind}_BLAKE2B" => "d" * 128, "#{kind}_SRI" => "sha256-#{'A' * 43}="
      )
    end
    result
  end

end
