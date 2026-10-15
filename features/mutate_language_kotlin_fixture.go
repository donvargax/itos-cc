// Build fixtures preserve the real pinned Kotlin/JaCoCo contracts.
package features

var languageKotlinBuilds = map[string]map[string]string{
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
