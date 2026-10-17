package main

// The Kotlin builds other tests share, from the Kotlin example of "Rule: A
// file's own tests are the tests that reach it, in every language" in
// features/mutate.feature, whose ID-MUT-217 holds own-tests-kotlin's old test
// whole (T-101). Its project's test classes each append their name and the
// directory they ran in to a log through Record, a test-support object in
// src/test that declares no test, so the steps see which test classes each
// command ran: coverage runs in the project's tree, and the baseline and
// mutants in worker copies, which are gone once the run returns. ATest, in
// A.kt's package, calls low without importing it; BTest reaches only B.kt
// and fails; and CTest reaches no file the graph sees: it calls A.kt's
// spare by reflection, by names it spells in strings, the way a test that
// runs code through a subprocess or the CLI executes code it names in no
// import or reference. It needs a real JDK with Gradle, or Maven, and skips,
// naming the missing one, without them; CI runs it on ubuntu-latest (T-13).

// kotlinOwnBuilds are the build files of the project by build tool, each
// measuring coverage with JaCoCo at a pinned version.
var kotlinOwnBuilds = map[string]map[string]string{
	"gradle": {
		"settings.gradle.kts": "rootProject.name = \"own\"\n",
		"build.gradle.kts": "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n    jacoco\n}\n\n" +
			"repositories {\n    mavenCentral()\n}\n\n" +
			"dependencies {\n    testImplementation(kotlin(\"test\"))\n" +
			"    testRuntimeOnly(\"org.junit.platform:junit-platform-launcher\")\n}\n\n" +
			"jacoco {\n    toolVersion = \"0.8.15\"\n}\n\n" +
			"tasks.test {\n    useJUnitPlatform()\n}\n\n" +
			"tasks.jacocoTestReport {\n    reports {\n        xml.required = true\n    }\n}\n",
	},
	"maven": {
		"pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>own</groupId>
  <artifactId>own</artifactId>
  <version>0.0.0</version>
  <properties>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    <kotlin.version>2.4.21</kotlin.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>org.jetbrains.kotlin</groupId>
      <artifactId>kotlin-stdlib</artifactId>
      <version>${kotlin.version}</version>
    </dependency>
    <dependency>
      <groupId>org.jetbrains.kotlin</groupId>
      <artifactId>kotlin-test-junit5</artifactId>
      <version>${kotlin.version}</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
  <build>
    <sourceDirectory>src/main/kotlin</sourceDirectory>
    <testSourceDirectory>src/test/kotlin</testSourceDirectory>
    <plugins>
      <plugin>
        <groupId>org.jetbrains.kotlin</groupId>
        <artifactId>kotlin-maven-plugin</artifactId>
        <version>${kotlin.version}</version>
        <executions>
          <execution><id>compile</id><goals><goal>compile</goal></goals></execution>
          <execution><id>test-compile</id><goals><goal>test-compile</goal></goals></execution>
        </executions>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-surefire-plugin</artifactId>
        <version>3.5.4</version>
      </plugin>
      <plugin>
        <groupId>org.jacoco</groupId>
        <artifactId>jacoco-maven-plugin</artifactId>
        <version>0.8.15</version>
      </plugin>
    </plugins>
  </build>
</project>
`,
	},
}
